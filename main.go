package main

import (
	videos "HuaTug.com/kitex_gen/videos/videoservice"
	"log"
)

func main() {
	svr := videos.NewServer(new(VideoServiceImpl))

	err := svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
