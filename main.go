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
	mainGrid.SetOrientation(gtk.ORIENTATION_HORIZONTAL)

	nameGrid, _ := gtk.GridNew()
	nameGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	nameGrid.SetBorderWidth(10)
	currGrid, _ := gtk.GridNew()
	currGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	currGrid.SetBorderWidth(10)
	minGrid, _ := gtk.GridNew()
	minGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	minGrid.SetBorderWidth(10)
	maxGrid, _ := gtk.GridNew()
	maxGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	maxGrid.SetBorderWidth(10)

	mainGrid.Add(nameGrid)
	mainGrid.Add(currGrid)
	mainGrid.Add(minGrid)
	mainGrid.Add(maxGrid)

	empty, _ := gtk.LabelNew("")
	current, _ := gtk.LabelNew("Current")
	currGrid.Add(current)
	min, _ := gtk.LabelNew("Min")
	minGrid.Add(min)
	max, _ := gtk.LabelNew("Max")
	maxGrid.Add(max)
	nameGrid.Add(empty)
	// cpuGrid, _ := gtk.GridNew()
	// cpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	if cpuinfo.PopulateContainer(nameGrid, currGrid, minGrid, maxGrid) {
		log.Println("Populated CPU grid")
		// mainGrid.Add(cpuGrid)
		// empty, _ := gtk.LabelNew("")
		// mainGrid.Add(empty)
		log.Println("CPU grid added")
	}
	nameGrid.Add(empty)
	currGrid.Add(empty)

	// gpuGrid, _ := gtk.GridNew()
	// gpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	if nvidia.PopulateContainer(nameGrid, currGrid) {
		log.Println("Populated GPU grid")
		// mainGrid.Add(gpuGrid)
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
