//go:build ignore

package main

import (
	"fmt"
	"os"
)

func main() {
	for _, v := range os.Environ() {
		fmt.Println(v)
	}
}
