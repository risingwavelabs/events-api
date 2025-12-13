package main

import (
	"log"

	"github.com/risingwavelabs/eventapi/wire"
)

func main() {
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
