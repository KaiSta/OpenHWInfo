package cpuinfo

import (
	"bytes"
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
		} else if strings.Contains(line, "Core") || strings.Contains(line, "Package") {
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
		}
	}

	log.Println("intel infos END")
	for k, v := range result {
		println(k, v)
	}
	return "unknown", result
}
