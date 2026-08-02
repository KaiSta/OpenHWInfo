package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
)

type Data struct {
	CpuKeys   []KeyTemplate
	GpuKeys   []KeyTemplate
	CpuValues map[string]string
	GpuValues map[string]string
}

type KeyTemplate struct {
	Label string
	Key   string
}

var (
	CPUVendorID = KeyTemplate{"CPU Vendor ID", "cpu.vendorID"}
	CPUBrand    = KeyTemplate{"CPU Brand", "cpu.brand"}
	CPUModel    = KeyTemplate{"CPU Model", "cpu.model"}
)

var cpuTemplates = []KeyTemplate{
	CPUVendorID,
	CPUBrand,
	CPUModel,
	{"CPU Clock", "cpu.clock0"},
	{"CPU Clock 1", "cpu.clock1"},
	{"CPU Clock 2", "cpu.clock2"},
	{"CPU Clock 3", "cpu.clock3"},
	{"CPU Temperature", "cpu.temp"},
	{"CPU PCH Temperature", "cpu.pch"},
}

var (
	GPUName          = KeyTemplate{"GPU Name", "name"}
	GPUDriverVersion = KeyTemplate{"Driver Version", "driverVersion"}
)

var gpuTemplates = []KeyTemplate{
	GPUName,
	GPUDriverVersion,
	{"GPU Temperature", "gpu.temp"},
	{"GPU Hotspot Temperature", "gpu.hotspot_temp"},
	{"VRAM Temperature", "vram.temp"},
	{"GPU Clock", "gpu.frequency"},
	{"VRAM Frequency", "vram.frequency"},
	{"VRAM Total", "vram.total"},
	{"VRAM Used", "vram.used"},
	{"VRAM Vendor", "vram.vendor"},
	{"GPU Usage", "gpu.usage"},
	{"GPU Power Max", "gpu.power.max"},
	{"GPU Power Avg", "gpu.power.avg"},
	{"Fan Speed", "fan.speed"},
	// Add more entries as needed...
}

var templateCache *template.Template

func init() {
	templateCache = template.Must(template.ParseGlob("website/*.html"))
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var cpuKeys []KeyTemplate = cpuTemplates
		var gpuKeys []KeyTemplate = gpuTemplates

		_, cpuElements := cpuinfo.GetInformations()
		_, gpuElements := amd.GetInformations()
		data := Data{
			CpuKeys:   cpuKeys,
			GpuKeys:   gpuKeys,
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
