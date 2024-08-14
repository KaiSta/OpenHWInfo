package main

import (
	"fmt"
	"time"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/nvidia"
)

func main() {
	for {
		_, cpuElements := cpuinfo.GetInformations()
		_, gpuElements := nvidia.GetInformations()
		_, gpu2Elements := amd.GetInformations()

		time.Sleep(5 * time.Second)

		fmt.Println("CPU Infos:")
		for key, value := range cpuElements {
			fmt.Println(key, value)
		}
		fmt.Println("GPU Infos:")
		for key, value := range gpuElements {
			fmt.Println(key, value)
		}
		fmt.Println("GPU2 Infos:")
		for key, value := range gpu2Elements {
			fmt.Println(key, value)
		}
		fmt.Println("-------------------------------------------------")
	}
}
