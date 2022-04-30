package main

import (
	"log"
	"time"

	"github.com/KaiSta/OpenHWInfo/nvidia"
	"github.com/gotk3/gotk3/gtk"
)

type hardwareInfos struct {
	cpuName           string
	cpuClock          string
	cpuTemp           string
	cpuTemp2          string
	cpuUtil           string
	coreClocks        []string
	memoryMax         string
	memoryCurrent     string
	gpuName           string
	gpuClock          string
	gpuMemClock       string
	gpuTemp           string
	gpuPowerDraw      string
	gpuPowerDrawMax   string
	gpuPowerDrawLimit string
	gpuUtil           string
	gpuMemTemp        string
	gpuMemTotal       string
	gpuMemCurrent     string
	gpuFanSpeed       string
}

type guiFields struct {
	cpuName           *gtk.Label
	cpuClock          *gtk.Label
	cpuTemp           *gtk.Label
	cpuTemp2          *gtk.Label
	cpuUtil           *gtk.Label
	coreClocks        []*gtk.Label
	memoryMax         *gtk.Label
	memoryCurrent     *gtk.Label
	gpuName           *gtk.Label
	gpuClock          *gtk.Label
	gpuMemClock       *gtk.Label
	gpuTemp           *gtk.Label
	gpuPowerDraw      *gtk.Label
	gpuPowerDrawMax   *gtk.Label
	gpuPowerDrawLimit *gtk.Label
	gpuUtil           *gtk.Label
	gpuMemTemp        *gtk.Label
	gpuMemTotal       *gtk.Label
	gpuMemCurrent     *gtk.Label
	gpuFanSpeed       *gtk.Label
}

func updateInfos() hardwareInfos {
	var hwdInfos hardwareInfos

	// GPU
	name, values := nvidia.GetInformations()
	hwdInfos.gpuName = name
	hwdInfos.gpuClock = values["gpu.clock"]
	hwdInfos.gpuMemClock = values["memory.clock"]
	hwdInfos.gpuTemp = values["gpu.temp"]
	hwdInfos.gpuPowerDraw = values["gpu.power.draw"]
	hwdInfos.gpuPowerDrawMax = values["gpu.power.max"]
	hwdInfos.gpuPowerDrawLimit = values["gpu.power.limit"]
	hwdInfos.gpuUtil = values["gpu.usage"]
	hwdInfos.gpuMemTemp = values["memory.temp"]
	hwdInfos.gpuMemTotal = values["memory.total"]
	hwdInfos.gpuFanSpeed = values["fan.speed"]
	hwdInfos.gpuMemCurrent = values["memory.used"]

	return hwdInfos
}

func createRow(name string, item *gtk.Label) *gtk.Grid {
	rowGrid, _ := gtk.GridNew()
	rowGrid.SetOrientation(gtk.ORIENTATION_HORIZONTAL)

	nameLabel, _ := gtk.LabelNew(name)
	rowGrid.Add(nameLabel)
	rowGrid.Add(item)
	return rowGrid
}

func main() {
	var fields guiFields

	// Initialize GTK without parsing any command line arguments.
	gtk.Init(nil)

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

	mainGrid, err := gtk.GridNew()
	if err != nil {
		log.Fatal("Unable to create grid:", err)
	}
	mainGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)

	fields.cpuName, err = gtk.LabelNew("CPUNAME")
	fields.cpuClock, err = gtk.LabelNew("CPUCLOCK")
	fields.cpuTemp, err = gtk.LabelNew("CPUTEMP")
	fields.cpuTemp2, err = gtk.LabelNew("CPUTEMP2")
	fields.cpuUtil, err = gtk.LabelNew("CPUUTIL")
	fields.memoryMax, err = gtk.LabelNew("MEMORYMAX")
	fields.memoryCurrent, err = gtk.LabelNew("MEMORYCURRENT")
	fields.gpuName, err = gtk.LabelNew("GPUNAME")
	fields.gpuClock, err = gtk.LabelNew("GPUCLOCK")
	fields.gpuMemClock, err = gtk.LabelNew("GPUMEMCLOCK")
	fields.gpuTemp, err = gtk.LabelNew("GPUTEMP")
	fields.gpuPowerDraw, err = gtk.LabelNew("GPUPOWERDRAW")
	fields.gpuPowerDrawMax, err = gtk.LabelNew("GPUPOWERDRAWMAX")
	fields.gpuPowerDrawLimit, err = gtk.LabelNew("GPUPOWERDRAWLIMIT")
	fields.gpuUtil, err = gtk.LabelNew("GPUUTIL")
	fields.gpuMemTemp, err = gtk.LabelNew("GPUMEMTEMP")
	fields.gpuMemCurrent, err = gtk.LabelNew("GPUMEMCURRENT")
	fields.gpuMemTotal, err = gtk.LabelNew("GPUMEMTOTAL")
	fields.gpuFanSpeed, err = gtk.LabelNew("GPUFANSPEED")

	cpuGrid, err := gtk.GridNew()
	cpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	cpuGrid.Add(fields.cpuName)
	cpuGrid.Add(createRow("Clock:", fields.cpuClock))
	cpuGrid.Add(createRow("Temp:", fields.cpuTemp))
	cpuGrid.Add(createRow("CCD Temp:", fields.cpuTemp2))
	cpuGrid.Add(createRow("Util:", fields.cpuUtil))
	// TODO Core Clocks list

	memGrid, err := gtk.GridNew()
	memGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	memLabel, _ := gtk.LabelNew("Memory")
	memGrid.Add(memLabel)
	memGrid.Add(createRow("Usage:", fields.memoryCurrent))
	memGrid.Add(createRow("Max:", fields.memoryMax))

	gpuGrid, err := gtk.GridNew()
	gpuGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	gpuGrid.Add(fields.gpuName)
	gpuGrid.Add(createRow("Clock:", fields.gpuClock))
	gpuGrid.Add(createRow("MemClock:", fields.gpuMemClock))
	gpuGrid.Add(createRow("Temp:", fields.gpuTemp))
	gpuGrid.Add(createRow("Util:", fields.gpuUtil))
	gpuGrid.Add(createRow("PowerDraw:", fields.gpuPowerDraw))
	gpuGrid.Add(createRow("PowerMax:", fields.gpuPowerDrawMax))
	gpuGrid.Add(createRow("PowerLimit:", fields.gpuPowerDrawLimit))
	gpuGrid.Add(createRow("Mem Temp:", fields.gpuMemTemp))
	gpuGrid.Add(createRow("Mem Current:", fields.gpuMemCurrent))
	gpuGrid.Add(createRow("Mem Total:", fields.gpuMemTotal))
	gpuGrid.Add(createRow("Fan Speed:", fields.gpuFanSpeed))

	empty, err := gtk.LabelNew("")
	empty2, err := gtk.LabelNew("")

	mainGrid.Add(cpuGrid)
	mainGrid.Add(empty)
	mainGrid.Add(memGrid)
	mainGrid.Add(empty2)
	mainGrid.Add(gpuGrid)

	win.Add(mainGrid)

	// Create a new label widget to show in the window.
	go func() {
		for true {
			infos := updateInfos()
			//GPU
			fields.gpuName.SetText(infos.gpuName)
			fields.gpuClock.SetText(infos.gpuClock)
			fields.gpuMemClock.SetText(infos.gpuMemClock)
			fields.gpuPowerDraw.SetText(infos.gpuPowerDraw)
			fields.gpuPowerDrawMax.SetText(infos.gpuPowerDrawMax)
			fields.gpuPowerDrawLimit.SetText(infos.gpuPowerDrawLimit)
			fields.gpuTemp.SetText(infos.gpuTemp)
			fields.gpuUtil.SetText(infos.gpuUtil)
			fields.gpuFanSpeed.SetText(infos.gpuFanSpeed)
			fields.gpuMemTemp.SetText(infos.gpuMemTemp)
			fields.gpuMemCurrent.SetText(infos.gpuMemCurrent)
			fields.gpuMemTotal.SetText(infos.gpuMemTotal)

			time.Sleep(3 * time.Second)
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
