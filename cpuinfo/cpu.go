package cpuinfo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"strings"
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
	for _, line := range lines {
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
		}
	}

	log.Println("intel infos END")

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
