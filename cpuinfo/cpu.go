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
		} else if strings.Contains(line, "Core") || strings.Contains(line, "Package") || strings.Contains(line, "Tccd") || strings.Contains(line, "Tdie") {
			elements := strings.Split(line, ":")
			temp := ""
			for _, c := range elements[1] {
				temp += string(c)
				if c == 'C' {
					break
				}
			}
			if !strings.HasSuffix(temp, "C") {
				temp += "C"
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

func parseTemp(temp string, def float64) float64 {
	log.Println("parsetemp", temp, def)
	temp = strings.TrimRight(temp, " ")
	temp = strings.Replace(temp, "°C", "", -1)

	if s, err := strconv.ParseFloat(temp, 64); err == nil {
		log.Println("parsetemp val", s)
		return s
	} else {
		log.Println("parsetemp error", err)
	}
	log.Println(def)
	return def
}

var minMap map[string]float64
var maxMap map[string]float64

func init() {
	minMap = make(map[string]float64)
	maxMap = make(map[string]float64)
}

func getMin(key string, value float64, unit string) string {
	if currMin, ok := minMap[key]; ok && currMin < value {
		return fmt.Sprintf("%v%v", currMin, unit)
	}
	minMap[key] = value
	return fmt.Sprintf("%v%v", value, unit)
}

func getMax(key string, value float64, unit string) string {
	if currMax, ok := maxMap[key]; ok && currMax > value {
		return fmt.Sprintf("%v%v", currMax, unit)
	}
	maxMap[key] = value
	return fmt.Sprintf("%v%v", value, unit)
}

var labelMap map[string]*gtk.Label
var labelMinMap map[string]*gtk.Label
var labelMaxMap map[string]*gtk.Label

func PopulateContainer(nameGrid, currGrid, minGrid, maxGrid *gtk.Grid) bool {
	log.Println("Starting to populate cpu grid")
	name, elements := GetInformations()

	if len(name) == 0 {
		return false
	}

	labelMap = make(map[string]*gtk.Label)
	labelMinMap = make(map[string]*gtk.Label)
	labelMaxMap = make(map[string]*gtk.Label)

	nameLabel, _ := gtk.LabelNew(name)
	nameGrid.Add(nameLabel)
	empty, _ := gtk.LabelNew("")
	currGrid.Add(empty)
	empty2, _ := gtk.LabelNew("")
	minGrid.Add(empty2)
	empty3, _ := gtk.LabelNew("")
	maxGrid.Add(empty3)

	maxClock := getMaxClock(elements)
	cpuClock, _ := gtk.LabelNew(maxClock)
	minCpuClock, _ := gtk.LabelNew(maxClock)
	maxCpuClock, _ := gtk.LabelNew(maxClock)
	labelClock, _ := gtk.LabelNew("Clock")
	labelMap["Clock"] = cpuClock
	labelMinMap["Clock"] = minCpuClock
	labelMaxMap["Clock"] = maxCpuClock
	nameGrid.Add(labelClock)
	currGrid.Add(cpuClock)
	minGrid.Add(minCpuClock)
	maxGrid.Add(maxCpuClock)

	cpuTemp, _ := gtk.LabelNew(elements["cpu.temp"])
	cpuMinTemp, _ := gtk.LabelNew(elements["cpu.temp"])
	cpuMaxTemp, _ := gtk.LabelNew(elements["cpu.temp"])
	labelTemp, _ := gtk.LabelNew("Temp")
	labelMap["Temp"] = cpuTemp
	labelMinMap["Temp"] = cpuMinTemp
	labelMaxMap["Temp"] = cpuMaxTemp
	nameGrid.Add(labelTemp)
	currGrid.Add(cpuTemp)
	minGrid.Add(cpuMinTemp)
	maxGrid.Add(cpuMaxTemp)

	count := 0
	clockCount := 0
	var coreNames []*gtk.Label
	var coreValues []*gtk.Label
	var coreMinValues []*gtk.Label
	var coreMaxValues []*gtk.Label
	var clockNames []*gtk.Label
	var clockValues []*gtk.Label
	var clockMinValues []*gtk.Label
	var clockMaxValues []*gtk.Label

	for k, x := range elements {
		if strings.Contains(k, "Package") {
			packageTemp, _ := gtk.LabelNew(x)
			packageMinTemp, _ := gtk.LabelNew(x)
			packageMaxTemp, _ := gtk.LabelNew(x)
			labelMap["Package"] = packageTemp
			labelMinMap["Package"] = packageMinTemp
			labelMaxMap["Package"] = packageMaxTemp
			labelPackage, _ := gtk.LabelNew("Package")
			nameGrid.Add(labelPackage)
			currGrid.Add(packageTemp)
			minGrid.Add(packageMinTemp)
			maxGrid.Add(packageMaxTemp)
		} else if strings.Contains(k, "ccd") {
			ccdTemp, _ := gtk.LabelNew(x)
			ccdMinTemp, _ := gtk.LabelNew(x)
			ccdMaxTemp, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("CCD %d", count)
			labelMap[labelName] = ccdTemp
			labelMinMap[labelName] = ccdMinTemp
			labelMaxMap[labelName] = ccdMaxTemp
			labelCCD, _ := gtk.LabelNew(labelName)
			nameGrid.Add(labelCCD)
			currGrid.Add(ccdTemp)
			minGrid.Add(ccdMinTemp)
			maxGrid.Add(ccdMaxTemp)
			count++
		} else if strings.Contains(k, "Core") {
			coreTemp, _ := gtk.LabelNew(x)
			coreMinTemp, _ := gtk.LabelNew(x)
			coreMaxTemp, _ := gtk.LabelNew(x)
			els := strings.Split(k, ".")
			labelMap[els[1]] = coreTemp
			labelMinMap[els[1]] = coreMinTemp
			labelMaxMap[els[1]] = coreMaxTemp
			labelName, _ := gtk.LabelNew(els[1])
			coreNames = append(coreNames, labelName)
			coreValues = append(coreValues, coreTemp)
			coreMinValues = append(coreMinValues, coreMinTemp)
			coreMaxValues = append(coreMaxValues, coreMaxTemp)
			str, _ := cpuTemp.GetText()
			if str == "" {
				cpuTemp.SetText(x)
			}
		} else if strings.Contains(k, "clock") {
			clock, _ := gtk.LabelNew(x)
			clockMin, _ := gtk.LabelNew(x)
			clockMax, _ := gtk.LabelNew(x)
			labelName := fmt.Sprintf("Clock %d", clockCount)
			labelMap[labelName] = clock
			labelMinMap[labelName] = clockMin
			labelMaxMap[labelName] = clockMax
			label, _ := gtk.LabelNew(labelName)
			clockNames = append(clockNames, label)
			clockValues = append(clockValues, clock)
			clockMinValues = append(clockMinValues, clockMin)
			clockMaxValues = append(clockMaxValues, clockMax)
			clockCount++
		}
	}
	for _, l := range coreNames {
		nameGrid.Add(l)
	}
	for _, l := range coreValues {
		currGrid.Add(l)
	}
	for _, l := range coreMinValues {
		minGrid.Add(l)
	}
	for _, l := range coreMaxValues {
		maxGrid.Add(l)
	}
	for _, l := range clockNames {
		nameGrid.Add(l)
	}
	for _, l := range clockValues {
		currGrid.Add(l)
	}
	for _, l := range clockMinValues {
		minGrid.Add(l)
	}
	for _, l := range clockMaxValues {
		maxGrid.Add(l)
	}

	if el, ok := elements["cpu.pch"]; ok {
		chipsetTemp, _ := gtk.LabelNew(el)
		chipsetMinTemp, _ := gtk.LabelNew(el)
		chipsetMaxTemp, _ := gtk.LabelNew(el)
		labelMap["PCH"] = chipsetTemp
		labelMinMap["PCH"] = chipsetMinTemp
		labelMaxMap["PCH"] = chipsetMaxTemp
		label, _ := gtk.LabelNew("PCH")
		nameGrid.Add(label)
		currGrid.Add(chipsetTemp)
		minGrid.Add(chipsetMinTemp)
		maxGrid.Add(chipsetMaxTemp)
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
			minEntry := labelMinMap[k]
			minTmp := getMin(k, parseTemp(elements["cpu.pch"], 999.99), "°C")
			minEntry.SetText(minTmp)
			maxEntry := labelMaxMap[k]
			maxTmp := getMax(k, parseTemp(elements["cpu.pch"], 0), "°C")
			maxEntry.SetText(maxTmp)
		} else if strings.Contains(k, "Clock") {
			els := strings.Split(k, " ")
			if len(els) == 1 {
				tmp := getMaxClock(elements)
				v.SetText(tmp)
				minEntry := labelMinMap[k]
				minTmp := getMin(k, parseTemp(tmp, 999.99), "")
				minEntry.SetText(minTmp)
				maxEntry := labelMaxMap[k]
				maxTmp := getMax(k, parseTemp(tmp, 0), "")
				maxEntry.SetText(maxTmp)
			} else {
				key := fmt.Sprintf("cpu.clock%s", els[1])
				v.SetText(elements[key])
				minEntry := labelMinMap[k]
				minTmp := getMin(k, parseTemp(elements[key], 999.99), "")
				minEntry.SetText(minTmp)
				maxEntry := labelMaxMap[k]
				maxTmp := getMax(k, parseTemp(elements[key], 0), "")
				maxEntry.SetText(maxTmp)
			}
		} else if strings.Contains(k, "Temp") {
			if x, ok := elements["cpu.temp"]; ok {
				v.SetText(x)
				minEntry := labelMinMap[k]
				minTmp := getMin(k, parseTemp(elements["cpu.temp"], 999.99), "°C")
				minEntry.SetText(minTmp)
				maxEntry := labelMaxMap[k]
				maxTmp := getMax(k, parseTemp(elements["cpu.temp"], 0), "°C")
				maxEntry.SetText(maxTmp)
			} else {
				cpuTempLabel = v
			}
		} else if strings.Contains(k, "Package") {
			for x, y := range elements {
				if strings.Contains(x, "Package") {
					v.SetText(y)
					minEntry := labelMinMap[k]
					minTmp := getMin(k, parseTemp(elements[x], 999.99), "°C")
					minEntry.SetText(minTmp)
					maxEntry := labelMaxMap[k]
					maxTmp := getMax(k, parseTemp(elements[x], 0), "°C")
					maxEntry.SetText(maxTmp)
				}
			}
		} else if strings.Contains(k, "CCD") {
			els := strings.Split(k, " ")
			for x, y := range elements {
				if strings.Contains(x, "ccd") && strings.Contains(x, els[1]) {
					v.SetText(y)
					minEntry := labelMinMap[k]
					minTmp := getMin(k, parseTemp(elements[x], 999.99), "°C")
					minEntry.SetText(minTmp)
					maxEntry := labelMaxMap[k]
					maxTmp := getMax(k, parseTemp(elements[x], 0), "°C")
					maxEntry.SetText(maxTmp)
				}
			}
		} else if strings.Contains(k, "Core") {
			els := strings.Split(k, " ")
			for x, y := range elements {
				if strings.Contains(x, "Core") && strings.Contains(x, els[1]) {
					v.SetText(y)
					minEntry := labelMinMap[k]
					minTmp := getMin(k, parseTemp(elements[x], 999.99), "°C")
					minEntry.SetText(minTmp)
					maxEntry := labelMaxMap[k]
					maxTmp := getMax(k, parseTemp(elements[x], 0), "°C")
					maxEntry.SetText(maxTmp)
				}
				if cpuTempLabel != nil {
					cpuTempLabel.SetText(y)
					minEntry := labelMinMap[k]
					minTmp := getMin(k, parseTemp(elements[x], 999.99), "°C")
					minEntry.SetText(minTmp)
					maxEntry := labelMaxMap[k]
					maxTmp := getMax(k, parseTemp(elements[x], 0), "°C")
					maxEntry.SetText(maxTmp)
				}
			}
		}
	}

}
