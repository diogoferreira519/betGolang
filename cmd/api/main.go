package main

import (
	"github.com/diogo/golangbet/internal/fxmodules"
	"go.uber.org/fx"
)

func main() {
	fx.New(fxmodules.Module).Run()
}
