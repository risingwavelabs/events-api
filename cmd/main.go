package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/risingwavelabs/eventapi"
	"github.com/risingwavelabs/eventapi/wire"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(eventapi.Version)
		return
	}

	app, err := wire.InitApp()
	if err != nil {
		log.Fatal(err)
	}
	defer app.Shutdown()

	if err := app.Listen(); err != nil {
		log.Fatal(err)
	}
	log.Println("bye.")
}
