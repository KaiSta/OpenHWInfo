package cpuinfo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gotk3/gotk3/gtk"
)

func GetInformations() (string, map[string]string) {
	log.Println("CPU infos")
	var result = make(map[string]string)

	cmd := exec.Command("sensors")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	str := out.String()

	lines := strings.Split(str, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "CPU") {
			elements := strings.Split(line, ":")
			result["cpu.temp"] = strings.TrimLeft(elements[1], " +")
		} else if strings.Contains(line, "Core") || strings.Contains(line, "Package") || strings.Contains(line, "Tccd") {
			elements := strings.Split(line, ":")
			temp := ""
			for _, c := range elements[1] {
				if c == 'C' {
					break
				}
				temp += string(c)
			}
			result["cpu."+elements[0]] = strings.TrimLeft(temp, " +")
		} else if strings.HasPrefix(line, "Tctl") {
			elements := strings.Split(line, ":")
			result["cpu.temp"] = strings.TrimLeft(elements[1], " +")
		} else if strings.HasPrefix(line, "pch") {
			necessaryLine := lines[i+2]
			elements := strings.Split(necessaryLine, ":")
			result["cpu.pch"] = strings.TrimLeft(elements[1], " +")
		}
	}

	cpuinfos, err := ioutil.ReadFile("/proc/cpuinfo") // the file is inside the local directory
	if err != nil {
		log.Fatal("Reading /proc/cpuinfo failed:", err)
	}
	name := ""
	cpus := 0
	lines = strings.Split(string(cpuinfos), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			name = strings.Split(line, ":")[1]
		} else if strings.HasPrefix(line, "cpu MHz") {
			result[fmt.Sprintf("cpu.clock%d", cpus)] = strings.TrimLeft(strings.Split(line, ":")[1], " ")
			cpus++
		}
	}

	return name, result
}

func getMaxClock(elements map[string]string) string {
	var vals []float64
	for k, v := range elements {
		if strings.Contains(k, "cpu.clock") {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				vals = append(vals, n)
			} else {
				log.Println(err)
			}
		}
	}
	var maxVal float64
	for _, x := range vals {
		if x > maxVal {
			maxVal = x
		}
	}
	return strconv.FormatFloat(maxVal, 'f', 2, 64)
}

func createRow(name string, item *gtk.Label) *gtk.Grid {
	rowGrid, _ := gtk.GridNew()
	rowGrid.SetOrientation(gtk.ORIENTATION_HORIZONTAL)
	rowGrid.SetColumnSpacing(10)

	nameLabel, _ := gtk.LabelNew(name)
	rowGrid.Add(nameLabel)
	rowGrid.Add(item)
	return rowGrid
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

var labelMap map[string]*gtk.Label

func PopulateContainer(grid *gtk.Grid) bool {
	log.Println("Starting to populate cpu grid")
	name, elements := GetInformations()

	if len(name) == 0 {
		return false
	}

	labelMap = make(map[string]*gtk.Label)

	nameLabel, _ := gtk.LabelNew(name)
	grid.Add(nameLabel)

	cpuClock, _ := gtk.LabelNew(getMaxClock(elements))
	labelMap["Clock"] = cpuClock
	grid.Add(createRow("Clock", cpuClock))

	cpuTemp, _ := gtk.LabelNew(elements["cpu.temp"])
	labelMap["Temp"] = cpuTemp
	grid.Add(createRow("Temp", cpuTemp))

	count := 0
	clockCount := 0
	coreGrid, _ := gtk.GridNew()
	coreGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)
	clockGrid, _ := gtk.GridNew()
	clockGrid.SetOrientation(gtk.ORIENTATION_VERTICAL)

	for k, x := range elements {
		if strings.Contains(k, "Package") {
			packageTemp, _ := gtk.LabelNew(x)
			labelMap["Package"] = packageTemp
			grid.Add(createRow("Package", packageTemp))
		} else if strings.Contains(k, "ccd") {
			ccdTemp, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("CCD %d", count)
			labelMap[labelName] = ccdTemp
			grid.Add(createRow(labelName, ccdTemp))
			count++
		} else if strings.Contains(k, "Core") {
			coreTemp, _ := gtk.LabelNew(x)
			els := strings.Split(k, ".")
			labelMap[els[1]] = coreTemp
			coreGrid.Add(createRow(els[1], coreTemp))
		} else if strings.Contains(k, "clock") {
			clock, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("Clock %d", clockCount)
			labelMap[labelName] = clock
			clockGrid.Add(createRow(labelName, clock))
			clockCount++
		}
	}
	grid.Add(clockGrid)
	grid.Add(coreGrid)

	if el, ok := elements["cpu.pch"]; ok {
		chipsetTemp, _ := gtk.LabelNew(el)
		labelMap["PCH"] = chipsetTemp
		grid.Add(createRow("PCH", chipsetTemp))
	}

	return true
}

func Update() {
	name, elements := GetInformations()

	if len(name) == 0 {
		return
	}

	for k, v := range labelMap {
		if strings.Contains(k, "PCH") {
			v.SetText(elements["cpu.pch"])
		} else if strings.Contains(k, "Clock") {
			v.SetText(getMaxClock(elements))
		} else if strings.Contains(k, "Temp") {
			v.SetText(elements["cpu.temp"])
		} else if strings.Contains(k, "Package") {
			for x, y := range elements {
				if strings.Contains(x, "Package") {
					v.SetText(y)
				}
			}
		} else if strings.Contains(k, "CCD") {
			els := strings.Split(k, " ")
			for x, y := range elements {
				if strings.Contains(x, "ccd") && strings.Contains(x, els[1]) {
					v.SetText(y)
				}
			}
		} else if strings.Contains(k, "Core") {
			els := strings.Split(k, " ")
			for x, y := range elements {
				if strings.Contains(x, "Core") && strings.Contains(x, els[1]) {
					v.SetText(y)
				}
			}
		} else if strings.Contains(k, "Clock") {
			els := strings.Split(k, " ")
			for x, y := range elements {
				if strings.Contains(x, "Clock") && strings.Contains(x, els[1]) {
					v.SetText(y)
				}
			}
		}
	}

}
