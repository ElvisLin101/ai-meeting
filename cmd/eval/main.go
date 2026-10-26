package main

import (
	"encoding/json"
	"fmt"
	"os"

	"ai-meeting/internal/eval"
)

func main() {
	report := eval.RunEmbedded()
	raw, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(raw))
	if !report.OK() {
		os.Exit(1)
	}
}
