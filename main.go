package main

import (
	"llm-router/app"
	"llm-router/config"
)

func main() {
	c, err := config.LoadConfig("config.yaml")
	if err != nil {
		panic(err)
	}
	a := app.NewApp(c)
	a.Run()
}
