package main

import (
	"log"

	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	// Initialize GTK without parsing any command line arguments.
	gtk.Init(nil)

	// Create a new toplevel window, set its title, and connect it to the
	// "destroy" signal to exit the GTK main loop when it is destroyed.
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Unable to create window:", err)
	}
	win.SetTitle("Simple Example")
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	grid, err := gtk.GridNew()
	if err != nil {
		log.Fatal("Unable to create grid:", err)
	}
	grid.SetOrientation(gtk.ORIENTATION_VERTICAL)

	// Create a new label widget to show in the window.
	cpu, err := gtk.LabelNew("Ryzen 7 1800X")
	if err != nil {
		log.Fatal("Unable to create label:", err)
	}

	cpuFreq1, err := gtk.LabelNew("Core 1")
	if err != nil {
		log.Fatal("Unable to create label:", err)
	}
	cpuFreq2, err := gtk.LabelNew("Core 2")
	if err != nil {
		log.Fatal("Unable to create label:", err)
	}
	grid.Add(cpu)
	grid.Add(cpuFreq1)
	grid.Add(cpuFreq2)

	empty, err := gtk.LabelNew("")
	grid.Add(empty)

	gpu, err := gtk.LabelNew("Geforce 1080 Ti")
	if err != nil {
		log.Fatal("Unable to create label:", err)
	}
	grid.Add(gpu)

	name, values := nvidia.GetInformations()
	nameLabel, err := gtk.LabelNew(name)
	grid.Add(nameLabel)

	for key, val := range values {
		innerGrid, err := gtk.GridNew()
		if err != nil {
			log.Fatal("Unable to create grid:", err)
		}
		innerGrid.SetOrientation(gtk.ORIENTATION_HORIZONTAL)
		nameLabel, err := gtk.LabelNew(key)
		if err != nil {
			log.Fatal("Unable to create label:", err)
		}
		innerGrid.Add(nameLabel)
		valueLabel, err := gtk.LabelNew(val)
		if err != nil {
			log.Fatal("Unable to create label:", err)
		}
		innerGrid.Add(valueLabel)
		grid.Add(innerGrid)
	}

	win.Add(grid)

	// Set the default window size.
	win.SetDefaultSize(800, 600)

	// Recursively show all widgets contained in this window.
	win.ShowAll()

	// Begin executing the GTK main loop.  This blocks until
	// gtk.MainQuit() is run.
	gtk.Main()
}
