# audio

[![Build Status](https://travis-ci.org/toy80/audio.svg?branch=master)](https://travis-ci.org/toy80/audio)
[![GolangCI](https://golangci.com/badges/github.com/toy80/audio.svg)](https://golangci.com)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Ftoy80%2Faudio.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Ftoy80%2Faudio?ref=badge_shield)


An experimental golang audio library, provide an Ogg-Vorbis decoder and an OpenAL-based playback interface.
It was designed for a 3D game engine.

## Installation

```bash
$ go get github.com/toy80/audio/...
```

## Usage

### Ogg+Vorbis Decoding

set [github.com/toy80/audio/vorbis/example-ogg2wav](https://github.com/toy80/audio/blob/master/vorbis/example-ogg2wav/example-ogg2wav.go)

```golang
package main

import (
  // ...
  "github.com/toy80/audio/vorbis"
  "github.com/toy80/audio/wav"
)

// ...

func convert(name string) {
  //...
  f, err := vorbis.Open(name)
  if err != nil {
    fmt.Println(err)
    os.Exit(1)
  }
  defer f.Close()

  //...
  if err = wav.WriteFile(name+".wav", f); err != nil {
    fmt.Println(err)
    os.Exit(1)
  }
}

// ...

```

### Audio Playback

see [github.com/toy80/audio/aplay/example-play-wav](https://github.com/toy80/audio/blob/master/aplay/example-play-wav/example-play-wav.go)

```golang
package main

import (
  // ...
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

```

## License

```text
This is free and unencumbered software released into the public domain.

Anyone is free to copy, modify, publish, use, compile, sell, or
distribute this software, either in source code form or as a compiled
binary, for any purpose, commercial or non-commercial, and by any
means.

In jurisdictions that recognize copyright laws, the author or authors
of this software dedicate any and all copyright interest in the
software to the public domain. We make this dedication for the benefit
of the public at large and to the detriment of our heirs and
successors. We intend this dedication to be an overt act of
relinquishment in perpetuity of all present and future rights to this
software under copyright law.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM, DAMAGES OR
OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.

For more information, please refer to <http://unlicense.org>
```

[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Ftoy80%2Faudio.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Ftoy80%2Faudio?ref=badge_large)