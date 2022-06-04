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

	probeCmd := exec.Command("which", "sensors")
	var probeOut bytes.Buffer
	probeCmd.Stdout = &probeOut
	err := probeCmd.Run()
	log.Println("CPU probe result", probeOut.String())
	if probeOut.String() == "" {
		log.Println("CPU probe failed, please install lm-sensors package")
		return "", nil
	}

	var result = make(map[string]string)

	cmd := exec.Command("sensors")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
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

var labelMap map[string]*gtk.Label

func PopulateContainer(nameGrid, currGrid *gtk.Grid) bool {
	log.Println("Starting to populate cpu grid")
	name, elements := GetInformations()

	if len(name) == 0 {
		return false
	}

	labelMap = make(map[string]*gtk.Label)

	nameLabel, _ := gtk.LabelNew(name)
	nameGrid.Add(nameLabel)
	empty, _ := gtk.LabelNew("")
	currGrid.Add(empty)

	cpuClock, _ := gtk.LabelNew(getMaxClock(elements))
	labelClock, _ := gtk.LabelNew("Clock")
	labelMap["Clock"] = cpuClock
	nameGrid.Add(labelClock)
	currGrid.Add(cpuClock)

	cpuTemp, _ := gtk.LabelNew(elements["cpu.temp"])
	labelTemp, _ := gtk.LabelNew("Temp")
	labelMap["Temp"] = cpuTemp
	nameGrid.Add(labelTemp)
	currGrid.Add(cpuTemp)

	count := 0
	clockCount := 0
	var coreNames []*gtk.Label
	var coreValues []*gtk.Label
	var clockNames []*gtk.Label
	var clockValues []*gtk.Label

	for k, x := range elements {
		if strings.Contains(k, "Package") {
			packageTemp, _ := gtk.LabelNew(x)
			labelMap["Package"] = packageTemp
			labelPackage, _ := gtk.LabelNew("Package")
			nameGrid.Add(labelPackage)
			currGrid.Add(packageTemp)
		} else if strings.Contains(k, "ccd") {
			ccdTemp, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("CCD %d", count)
			labelMap[labelName] = ccdTemp
			labelCCD, _ := gtk.LabelNew(labelName)
			nameGrid.Add(labelCCD)
			currGrid.Add(ccdTemp)
			count++
		} else if strings.Contains(k, "Core") {
			coreTemp, _ := gtk.LabelNew(x)
			els := strings.Split(k, ".")
			labelMap[els[1]] = coreTemp
			labelName, _ := gtk.LabelNew(els[1])
			coreNames = append(coreNames, labelName)
			coreValues = append(coreValues, coreTemp)
			str, _ := cpuTemp.GetText()
			if str == "" {
				cpuTemp.SetText(x)
			}
		} else if strings.Contains(k, "clock") {
			clock, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("Clock %d", clockCount)
			labelMap[labelName] = clock
			label, _ := gtk.LabelNew(labelName)
			clockNames = append(clockNames, label)
			clockValues = append(clockValues, clock)
			clockCount++
		}
	}
	for _, l := range coreNames {
		nameGrid.Add(l)
	}
	for _, l := range coreValues {
		currGrid.Add(l)
	}
	for _, l := range clockNames {
		nameGrid.Add(l)
	}
	for _, l := range clockValues {
		currGrid.Add(l)
	}

	if el, ok := elements["cpu.pch"]; ok {
		chipsetTemp, _ := gtk.LabelNew(el)
		labelMap["PCH"] = chipsetTemp
		label, _ := gtk.LabelNew("PCH")
		nameGrid.Add(label)
		currGrid.Add(chipsetTemp)
	}

	return true
}

func Update() {
	name, elements := GetInformations()

	if len(name) == 0 {
		return
	}
	var cpuTempLabel *gtk.Label
	for k, v := range labelMap {
		if strings.Contains(k, "PCH") {
			v.SetText(elements["cpu.pch"])
		} else if strings.Contains(k, "Clock") {
			els := strings.Split(k, " ")
			if len(els) == 1 {
				v.SetText(getMaxClock(elements))
			} else {
				log.Println("els[1]=", els[1])
				key := fmt.Sprintf("cpu.clock%s", els[1])
				log.Println("Searching for key ", key)
				v.SetText(elements[key])
			}
		} else if strings.Contains(k, "Temp") {
			if x, ok := elements["cpu.temp"]; ok {
				v.SetText(x)
			} else {
				cpuTempLabel = v
			}
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
				if cpuTempLabel != nil {
					cpuTempLabel.SetText(y)
				}
			}
		}
	}

}
