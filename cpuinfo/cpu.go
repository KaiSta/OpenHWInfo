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
	log.Println("intel infos")
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
			println("elements[1]", elements[1])
			for _, c := range elements[1] {
				if c == 'C' {
					break
				}
				temp += string(c)
			}
			print(temp)
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
			result[fmt.Sprintf("cpu.clock%d", cpus)] = strings.Split(line, ":")[1]
			cpus++
		}
	}

	return name, result
}

func getMaxClock(elements map[string]string) string {
	var vals []float64
	for k, v := range elements {
		if strings.HasPrefix(k, "cpu.clock") {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				vals = append(vals, n)
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

	nameLabel, _ := gtk.LabelNew(name)
	rowGrid.Add(nameLabel)
	rowGrid.Add(item)
	return rowGrid
}

func clearGrid(grid *gtk.Grid) {
	for true {
		_, err := grid.GetChildAt(0, 1)
		if err == nil {
			grid.RemoveRow(1)
		} else {
			return
		}
	}
}

func PopulateContainer(grid *gtk.Grid) bool {
	clearGrid(grid)
	name, elements := GetInformations()

	if len(name) == 0 {
		return false
	}
	nameLabel, _ := gtk.LabelNew(name)
	grid.Add(nameLabel)

	cpuClock, _ := gtk.LabelNew(getMaxClock(elements))
	grid.Add(createRow("Clock", cpuClock))

	cpuTemp, _ := gtk.LabelNew(elements["cpu.temp"])
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
			grid.Add(createRow("Package", packageTemp))
		} else if strings.Contains(k, "ccd") {
			ccdTemp, _ := gtk.LabelNew(x)
			grid.Add(createRow(fmt.Sprintf("CCD%d", count), ccdTemp))
			count++
		} else if strings.Contains(k, "Core") {
			coreTemp, _ := gtk.LabelNew(x)
			coreGrid.Add(createRow(k, coreTemp))
		} else if strings.Contains(k, "clock") {
			clock, _ := gtk.LabelNew(x)
			clockGrid.Add(createRow(fmt.Sprintf("Clock %d", clockCount), clock))
			clockCount++
		}
	}
	grid.Add(clockGrid)
	grid.Add(coreGrid)

	if el, ok := elements["cpu.pch"]; ok {
		chipsetTemp, _ := gtk.LabelNew(el)
		grid.Add(createRow("PCH", chipsetTemp))
	}

	return true
}
