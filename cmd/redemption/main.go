package main

import (
	"log"

	"spacetime-node/internal/platform/app"
)

var version = "dev"

func main() {
	if err := app.Run("redemption-service", ":8003", ":9003", version); err != nil {
		log.Fatal(err)
	}
}
