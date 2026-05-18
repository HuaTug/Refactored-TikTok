// P01 · 直接通过 Kitex 客户端压测 InteractionService.LikeAction
// 不走 HTTP 网关，纯测 RPC 框架开销。
//
//go:build perf
// +build perf

package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/interactions/interactionservice"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/transport"
)

func main() {
	var (
		target      = flag.String("target", "127.0.0.1:8893", "Interaction kitex addr")
		concurrency = flag.Int("c", 100, "concurrent goroutines")
		duration    = flag.Duration("duration", 5*time.Minute, "run duration")
		startVid    = flag.Int64("vid", 9000001, "starting video_id")
		startUid    = flag.Int64("uid", 1000000, "starting user_id")
	)
	flag.Parse()

	cli, err := interactionservice.NewClient(
		"Interaction",
		client.WithHostPorts(*target),
		client.WithRPCTimeout(2*time.Second),
		client.WithMuxConnection(2), // 服务端启用了 WithMuxTransport，客户端必须对齐
		client.WithTransportProtocol(transport.TTHeader),
	)
	if err != nil {
		panic(err)
	}

	var (
		ok, fail uint64
		latMu    sync.Mutex
		samples  = make([]int64, 0, 5_000_000) // 微秒
	)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed) + time.Now().UnixNano()))
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				vid := *startVid + r.Int63n(50) + 1 // 真实 video_id 1~50
				if vid > 129 {
					vid = (vid % 129) + 1
				}
				uid := *startUid + r.Int63n(100000)
				t0 := time.Now()
				_, err := cli.LikeAction(context.Background(), &interactions.LikeActionRequest{
					UserId:     uid,
					VideoId:    vid,
					ActionType: "like", // 业务层接受 "like"/"unlike"
				})
				cost := time.Since(t0).Microseconds()
				latMu.Lock()
				samples = append(samples, cost)
				latMu.Unlock()
				if err != nil {
					atomic.AddUint64(&fail, 1)
				} else {
					atomic.AddUint64(&ok, 1)
				}
			}
		}(w)
	}

	wg.Wait()

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	pct := func(p float64) float64 {
		if len(samples) == 0 {
			return 0
		}
		idx := int(float64(len(samples)-1) * p)
		return float64(samples[idx]) / 1000.0 // ms
	}
	total := ok + fail
	qps := float64(total) / duration.Seconds()
	fmt.Printf("framework=kitex target=%s c=%d duration=%s\n", *target, *concurrency, *duration)
	fmt.Printf("total=%d ok=%d fail=%d  QPS=%.0f\n", total, ok, fail, qps)
	fmt.Printf("TP50=%.2fms  TP90=%.2fms  TP99=%.2fms  TP999=%.2fms  MAX=%.2fms\n",
		pct(0.50), pct(0.90), pct(0.99), pct(0.999),
		float64(samples[len(samples)-1])/1000.0)
}
