package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/KaiSta/OpenHWInfo/amd"
	"github.com/KaiSta/OpenHWInfo/cpuinfo"
)

type Data struct {
	CpuKeys   []string
	GpuKeys   []string
	CpuValues map[string]string
	GpuValues map[string]string
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var cpuKeys []string
		var gpuKeys []string

		_, cpuElements := cpuinfo.GetInformations()
		if len(cpuKeys) == 0 {
			for key := range cpuElements {
				cpuKeys = append(cpuKeys, key)
			}
		}

		_, gpuElements := amd.GetInformations()
		if len(gpuKeys) == 0 {
			for key := range gpuElements {
				gpuKeys = append(gpuKeys, key)
			}
		}

		data := Data{
			CpuKeys:   cpuKeys,
			GpuKeys:   gpuKeys,
			CpuValues: cpuElements,
			GpuValues: gpuElements,
		}

		tmpl := template.Must(template.ParseGlob("website/*.html"))
		err := tmpl.ExecuteTemplate(w, "index.html", data)
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
