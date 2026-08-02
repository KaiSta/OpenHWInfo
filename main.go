package main

import (
	"fmt"
	"os"
	"time"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/olekukonko/tablewriter"
)

func main() {
	var cpuKeys []string
	var gpuKeys []string

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {

		_, cpuElements := cpuinfo.GetInformations()

		if len(cpuKeys) == 0 {
			for key := range cpuElements {
				cpuKeys = append(cpuKeys, key)
			}
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.Header("Element", "CPU Value")
		for _, key := range cpuKeys {
			value, ok := cpuElements[key]
			if !ok {
				value = ""
			}
			table.Append([]string{key, value})
		}

		_, gpuElements := amd.GetInformations()
		fmt.Println("GPU Elements:", gpuElements) // Debugging line to print GPU elements

		if len(gpuKeys) == 0 {
			for key := range gpuElements {
				gpuKeys = append(gpuKeys, key)
			}
		}

		table.Append([]string{"", ""}) // Empty row for separation

		table.Append([]string{"GPU Metrics", ""}) // Header for GPU metrics
		for _, key := range gpuKeys {
			value, ok := gpuElements[key]
			if !ok {
				value = ""
			}
			table.Append([]string{key, value})
		}

		table.Render()
	}
}
