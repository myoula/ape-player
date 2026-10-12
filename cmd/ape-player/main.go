package main

import (
	"ape-player/internal/app"
	"ape-player/internal/diagnostics"
	"flag"
	"log"
)

func main() {
	file := flag.String("play", "", "直接播放本地文件，用于验证 GPU 播放后端")
	smoke := flag.String("smoke", "", "运行本地多轨视频 GPU 验证后退出")
	diagnose := flag.String("diagnose", "", "使用当前 Emby 来源诊断指定 Item ID 的原画 GPU 播放，60 秒内退出")
	flag.Parse()
	closeLog, err := diagnostics.Init()
	if err != nil {
		log.Printf("无法创建播放日志：%v", err)
	} else {
		defer closeLog()
		log.Printf("播放诊断日志：%s", diagnostics.Path())
	}
	diagnostics.Log("application.start")
	a, e := app.New()
	if e != nil {
		log.Fatal(e)
	}
	if *diagnose != "" {
		e = a.Diagnose(*diagnose)
	} else if *smoke != "" {
		e = a.Smoke(*smoke)
	} else {
		e = a.Run(*file)
	}
	if e != nil {
		log.Fatal(e)
	}
}
