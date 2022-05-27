package nvidia

import (
	"bytes"
	"log"
	"os/exec"
	"strings"

	"github.com/gotk3/gotk3/gtk"
)

func GetInformations() (string, map[string]string) {
	log.Println("nvidia get infos")
	probeCmd := exec.Command("which", "nvidia-smi")
	var probeOut bytes.Buffer
	probeCmd.Stdout = &probeOut
	print("hel")
	err := probeCmd.Run()
	print("lo")
	log.Println("Probe result", probeOut.String())
	if probeOut.String() == "" {
		log.Println("Probe failed")
		return "", nil
	}

	cmd := exec.Command("nvidia-smi", "--query-gpu=name,temperature.gpu,memory.used,utilization.gpu,power.draw,clocks.current.graphics,clocks.current.memory,power.limit,power.max_limit,temperature.memory,memory.total,fan.speed", "--format=csv")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
		return "", nil
	}
	str := out.String()
	print(str)

	var data = make(map[string]string)

	splitStr := strings.Split(str, "\n")
	fields := strings.Split(splitStr[1], ",")

	data["gpu.temp"] = fields[1]
	data["memory.used"] = fields[2]
	data["gpu.usage"] = fields[3]
	data["gpu.power.draw"] = fields[4]
	data["gpu.clock"] = fields[5]
	data["memory.clock"] = fields[6]
	data["gpu.power.max"] = fields[7]
	data["gpu.power.limit"] = fields[8]
	data["memory.temp"] = fields[9]
	data["memory.total"] = fields[10]
	data["fan.speed"] = fields[11]

	return fields[0], data

}

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

func createRow(name string, item *gtk.Label) *gtk.Grid {
	rowGrid, _ := gtk.GridNew()
	rowGrid.SetOrientation(gtk.ORIENTATION_HORIZONTAL)

	nameLabel, _ := gtk.LabelNew(name)
	rowGrid.Add(nameLabel)
	rowGrid.Add(item)
	return rowGrid
}

var labelMap map[string]*gtk.Label

func PopulateContainer(grid *gtk.Grid) bool {
	name, elements := GetInformations()

	if len(name) == 0 {
		return false
	}

	labelMap = make(map[string]*gtk.Label)

	nameLabel, _ := gtk.LabelNew(name)
	grid.Add(nameLabel)

	if x, ok := elements["gpu.clock"]; ok {
		clockLabel, _ := gtk.LabelNew(x)
		labelMap["Clock"] = clockLabel
		grid.Add(createRow("Clock", clockLabel))
	}

	if x, ok := elements["memory.clock"]; ok {
		memLabel, _ := gtk.LabelNew(x)
		labelMap["RAM Clock"] = memLabel
		grid.Add(createRow("RAM Clock", memLabel))
	}

	if x, ok := elements["gpu.temp"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["Temp"] = l
		grid.Add(createRow("Temp", l))
	}

	if x, ok := elements["memory.temp"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["RAM Temp"] = l
		grid.Add(createRow("RAM Temp", l))
	}

	if x, ok := elements["gpu.power.draw"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["PowerDraw"] = l
		grid.Add(createRow("PowerDraw", l))
	}

	if x, ok := elements["gpu.power.max"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["PowerMax"] = l
		grid.Add(createRow("PowerMax", l))
	}

	if x, ok := elements["gpu.power.limit"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["PowerLimit"] = l
		grid.Add(createRow("PowerLimit", l))
	}

	if x, ok := elements["gpu.usage"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["Util"] = l
		grid.Add(createRow("Util", l))
	}

	if x, ok := elements["fan.speed"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["FanSpeed"] = l
		grid.Add(createRow("FanSpeed", l))
	}

	if x, ok := elements["memory.used"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["MemoryUsed"] = l
		grid.Add(createRow("MemoryUsed", l))
	}

	if x, ok := elements["memory.total"]; ok {
		l, _ := gtk.LabelNew(x)
		labelMap["MemoryTotal"] = l
		grid.Add(createRow("MemoryTotal", l))
	}

	return true
}

func Update() {
	name, elements := GetInformations()

	if len(name) == 0 {
		return
	}

	clock := labelMap["Clock"]
	clock.SetText(elements["gpu.clock"])

	ramClock := labelMap["RAM Clock"]
	ramClock.SetText(elements["memory.clock"])

	temp := labelMap["Temp"]
	temp.SetText(elements["gpu.temp"])

	ramTemp := labelMap["RAM Temp"]
	ramTemp.SetText(elements["memory.temp"])

	powerdraw := labelMap["PowerDraw"]
	powerdraw.SetText(elements["gpu.power.draw"])

	powerMax := labelMap["PowerMax"]
	powerMax.SetText(elements["gpu.power.max"])

	powerLimit := labelMap["PowerLimit"]
	powerLimit.SetText(elements["gpu.power.limit"])

	util := labelMap["Util"]
	util.SetText(elements["gpu.usage"])

	fanspeed := labelMap["FanSpeed"]
	fanspeed.SetText(elements["fan.speed"])

	memused := labelMap["MemoryUsed"]
	memused.SetText(elements["memory.used"])

	memtotal := labelMap["MemoryTotal"]
	memtotal.SetText(elements["memory.total"])
}
