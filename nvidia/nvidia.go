package nvidia

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
)

func GetInformations() (string, map[string]string) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,temperature.gpu,memory.used,utilization.gpu,power.draw,clocks.current.graphics,clocks.current.memory", "--format=csv")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	str := out.String()

	var data = make(map[string]string)

	splitStr := strings.Split(str, "\n")
	fields := strings.Split(splitStr[1], ",")

	data["temp"] = fields[1]
	data["memory.used"] = fields[2]
	data["gpu.usage"] = fields[3]
	data["power.draw"] = fields[4]
	data["gpu.clock"] = fields[5]
	data["memory.clock"] = fields[6]

	return fields[0], data

}
