package main

import (
	"log"
	"time"

	"github.com/KaiSta/OpenHWInfo/cpuinfo"
	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/gotk3/gotk3/gtk"
)

func clearGrid(grid *gtk.Grid) {
	for true {
		el, _ := grid.GetChildAt(0, 1)
		if el != nil {
			grid.RemoveRow(1)
		} else {
			return
		}
	}
}

func main() {

	// Initialize GTK without parsing any command line arguments.
	gtk.Init(nil)

	log.Println("Starting OpenHWInfo")

	// Create a new toplevel window, set its title, and connect it to the
	// "destroy" signal to exit the GTK main loop when it is destroyed.
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}
	win.SetTitle("OpenHWInfo")
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})
	win.SetBorderWidth(6)

	log.Println("Created Window")

	var mainGrid *gtk.Grid
	mainGrid, _ = gtk.GridNew()
	mainGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)

	cpuGrid, _ := gtk.GridNew()
	cpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	if cpuinfo.PopulateContainer(cpuGrid) {
		log.Println("Populated CPU grid")
		mainGrid.Add(cpuGrid)
		empty, _ := gtk.LabelNew("")
		mainGrid.Add(empty)
		log.Println("CPU grid added")
	}

	gpuGrid, _ := gtk.GridNew()
	gpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	if nvidia.PopulateContainer(gpuGrid) {
		log.Println("Populated GPU grid")
		mainGrid.Add(gpuGrid)
		empty, _ := gtk.LabelNew("")
		mainGrid.Add(empty)
		log.Println("GPU grid added")
	}

	win.Add(mainGrid)
	log.Println("Created main grid")

	// Create a new label widget to show in the window.
	go func() {
		for {
			cpuinfo.Update()
			nvidia.Update()

			time.Sleep(5 * time.Second)
		}
	}()

	// Set the default window size.
	win.SetDefaultSize(800, 600)

	// Recursively show all widgets contained in this window.
	win.ShowAll()

	// Begin executing the GTK main loop.  This blocks until
	// gtk.MainQuit() is run.
	gtk.Main()
}
