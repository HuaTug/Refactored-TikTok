// 批量登录由 seed_data.sql 灌入的 perf_u_xxxxxx 用户，导出 tokens.csv
// 用法：
//   go run scripts/perf/gen_tokens.go -n 1000 -out scripts/perf/tokens.csv
//   go run scripts/perf/gen_tokens.go -n 1000 -base http://localhost:8888 -concurrency 32

//go:build perf
// +build perf

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

type loginResp struct {
	Code int `json:"code"`
	Data struct {
		Base struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"base"`
		Token string `json:"token"`
		User  struct {
			UserID int64 `json:"user_id"`
		} `json:"user"`
	} `json:"data"`
	Message string `json:"message"`
}

type createResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func register(client *http.Client, base, user, pass string) error {
	body, _ := json.Marshal(map[string]any{
		"username": user, "password": pass,
		"email": user + "@perf.local", "sex": 1,
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/user/create/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var cr createResp
	_ = json.Unmarshal(raw, &cr)
	// 已存在的也当成功（重复注册返回 errno != 10000，但下一步 login 会成功）
	if cr.Code != 10000 {
		// 不阻断，继续尝试登录
	}
	return nil
}

func login(client *http.Client, base, user, pass string) (int64, string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": user,
		"password": pass,
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/user/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var lr loginResp
	if err := json.Unmarshal(raw, &lr); err != nil {
		return 0, "", fmt.Errorf("bad json: %s", string(raw))
	}
	// 业务真正成功：token 非空（base.code 取值有 0/200/10000 多种成功语义）
	if lr.Data.Token == "" {
		return 0, "", fmt.Errorf("login failed: base.code=%d msg=%s", lr.Data.Base.Code, lr.Data.Base.Msg)
	}
	return lr.Data.User.UserID, lr.Data.Token, nil
}

func main() {
	var (
		base        = flag.String("base", "http://localhost:8888", "API gateway")
		n           = flag.Int("n", 1000, "how many users to login")
		concurrency = flag.Int("concurrency", 32, "concurrent login workers")
		password    = flag.String("password", "123456", "password used for all perf users")
		prefix      = flag.String("prefix", "perf_login_", "username prefix; will register if missing")
		startIdx    = flag.Int("start", 0, "starting index appended to prefix")
		out         = flag.String("out", "scripts/perf/tokens.csv", "output csv path: user_id,token")
		ensureReg   = flag.Bool("register", true, "register user before login if not exists")
	)
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}
	jobs := make(chan int, *n)
	type row struct {
		uid   int64
		token string
		uname string
		err   error
	}
	results := make(chan row, *n)

	var wg sync.WaitGroup
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				uname := fmt.Sprintf("%s%06d", *prefix, *startIdx+i)
				if *ensureReg {
					_ = register(client, *base, uname, *password)
				}
				uid, tk, err := login(client, *base, uname, *password)
				results <- row{uid: uid, token: tk, uname: uname, err: err}
			}
		}()
	}

	for i := 0; i < *n; i++ {
		jobs <- i
	}
	close(jobs)

	go func() { wg.Wait(); close(results) }()

	f, err := os.Create(*out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, "user_id,username,token")

	ok, fail := 0, 0
	t0 := time.Now()
	for r := range results {
		if r.err != nil {
			fail++
			if fail <= 5 {
				fmt.Fprintf(os.Stderr, "[fail] %s: %v\n", r.uname, r.err)
			}
			continue
		}
		ok++
		fmt.Fprintf(f, "%d,%s,%s\n", r.uid, r.uname, r.token)
	}
	fmt.Printf("done: ok=%d fail=%d elapsed=%s file=%s\n",
		ok, fail, time.Since(t0).Round(time.Millisecond), *out)
}
