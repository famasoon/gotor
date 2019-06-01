// Copyright 2015 The GoTor Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/tvdw/cgolock"
)

import _ "net/http/pprof"
import _ "expvar"

func main() {
	// CGOでCPUの数以上のスレッドが生成されないよう抑制
	cgolock.Init(runtime.NumCPU())
	// マルチコアを使うように設定(最新のGoバージョンだといらないかもしれない)
	runtime.GOMAXPROCS(runtime.NumCPU())

	// 乱数生成器のシード決定
	SetupRand()

	// Cell作成
	SeedCellBuf()

	// config.goで定義されている
	torConfig := Config{
		IsPublicServer: true,
		// Torのバージョン
		Platform:          "Tor 0.2.6.2-alpha on Go",
		BandwidthAvg:      1073741824,
		BandwidthBurst:    1073741824,
		BandwidthObserved: 1 << 16,
	}
	// torrcを読み込んで値があったら設定をしている
	if err := torConfig.ReadFile(os.Args[1]); err != nil {
		log.Panicln(err)
	}

	// torrcから読み込んだ設定を基にorを作成
	or, err := NewOR(&torConfig)
	if err != nil {
		log.Panicln(err)
	}

	/*
		go func() {
			or.RequestCircuit(&CircuitRequest{
				localID: 5,
				connHint: ConnectionHint{
					address: [][]byte{[]byte{127,0,0,1,35,41}},
				},
			})
		}()
	*/

	anythingFinished := make(chan int)
	go func() {
		or.Run()
		// or.Run()が終了するまでこのgoroutineはロックされる
		// or.Run()内でもgoroutineで処理が実行されている
		// そのため何らかのエラーが発生しない限りはor.Run()は終了しない
		// 何かエラーが発生したらanythingFinishedチャネルに1を送る
		anythingFinished <- 1
	}()
	go func() {
		Log(LOG_WARN, "%v", http.ListenAndServe("localhost:6060", nil))
	}()

	// 作成したOR情報を外部公開
	or.PublishDescriptor()

	nextRotate := time.After(time.Hour * 1)

	// 次にDescriptorサーバに情報を送る時間
	nextPublish := time.After(time.Hour * 18)
	for {
		select {
		case <-nextRotate: //XXX randomer intervals
			if err := or.RotateKeys(); err != nil {
				Log(LOG_WARN, "%v", err)
			}
			nextRotate = time.After(time.Hour * 1)

		case <-nextPublish:
			or.PublishDescriptor()
			nextPublish = time.After(time.Hour * 18)

		// or.Run()の処理が終了しanythingFinishedに値が届く
		case <-anythingFinished:
			log.Panicln("Somehow a main.go goroutine we spawned managed to finish, which is not good")
		}
	}
}
