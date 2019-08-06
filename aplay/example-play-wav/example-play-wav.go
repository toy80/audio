package main

import (
	"log"
	"os"
	"time"

	"github.com/toy80/audio/aplay"
	"github.com/toy80/audio/wav"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalln("Usage: example-play-wav WAV_FILE")
	}
	defer aplay.WaitIdle()

	sound, err := wav.Open(os.Args[1])
	if err != nil {
		log.Fatalln(err)
	}
	defer sound.Close()

	player := aplay.Alloc()
	if err = player.Play(sound, 1, 0); err != nil {
		log.Fatalln(err)
	}

	time.Sleep(sound.Duration() + time.Second)
}
