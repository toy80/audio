package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/toy80/go-al/vorbis"
	"github.com/toy80/go-al/wav"
)

var cpuprofile = flag.String("p", "", "write cpu profile to file")

func convert(name string) {
	fmt.Println("read", name)

	f, err := vorbis.Open(name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Println("write", name+".wav")
	if err = wav.WriteFile(name+".wav", f); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Usage of %s:\n\n", name)
	fmt.Fprintf(os.Stderr, "  %s [-p pprof.out] foo.ogg bar.ogg other.ogg ...\n\n", name)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	for i := 0; i < flag.NArg(); i++ {
		convert(flag.Arg(i))
	}
	fmt.Println("done.")
}
