package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/shared"
)

type Data struct {
	CpuKeys   []shared.KeyTemplate
	GpuKeys   []shared.KeyTemplate
	CpuValues map[string]string
	GpuValues map[string]string
}

var cpuTemplates = []shared.KeyTemplate{
	shared.CPUVendorID,
	shared.CPUBrand,
	shared.CPUModel,
	shared.CPUClock0,
	shared.CPUClock1,
	shared.CPUClock2,
	shared.CPUClock3,
	shared.CPUTemp,
	shared.CPUPCHTemp,
	shared.TCcd0,
	shared.TCcd1,
	shared.TCcd2,
	shared.TCcd3,
	shared.TCcd4,
	shared.TCcd5,
}

var gpuTemplates = []shared.KeyTemplate{
	shared.GPUName,
	shared.GPUDriverVersion,
	shared.GPUTemp,
	shared.GPUHotspotTemp,
	shared.VRAMTemp,
	shared.GPUFrequency,
	shared.VRAMLrequency,
	shared.VRAMTotal,
	shared.VRAMUsed,
	shared.VRAMVendor,
	shared.GPUUsage,
	shared.GPUPowerMax,
	shared.GPUPowerAvg,
	shared.FanSpeed,
}

var templateCache *template.Template

func init() {
	templateCache = template.Must(template.ParseGlob("website/*.html"))
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, cpuElements := cpuinfo.GetInformations()
		_, gpuElements := amd.GetInformations()

		// Filter CPU keys to only show those with available values
		var filteredCpuKeys []shared.KeyTemplate
		for _, key := range cpuTemplates {
			if value, exists := cpuElements[key.Key]; exists && value != "" {
				filteredCpuKeys = append(filteredCpuKeys, key)
			}
		}

		// Filter GPU keys to only show those with available values
		var filteredGpuKeys []shared.KeyTemplate
		for _, key := range gpuTemplates {
			if value, exists := gpuElements[key.Key]; exists && value != "" {
				filteredGpuKeys = append(filteredGpuKeys, key)
			}
		}

		data := Data{
			CpuKeys:   filteredCpuKeys,
			GpuKeys:   filteredGpuKeys,
			CpuValues: cpuElements,
			GpuValues: gpuElements,
		}

		err := templateCache.ExecuteTemplate(w, "index.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	fs := http.FileServer(http.Dir("website/resources"))
	http.Handle("/resources/", http.StripPrefix("/resources/", fs))

	fmt.Println("Starting server at port 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic(err)
	}
}
