package nvidia

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
)

func GetInformations() (string, map[string]string) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,temperature.gpu,memory.used,utilization.gpu,power.draw,clocks.current.graphics,clocks.current.memory,power.limit,power.max_limit,temperature.memory,memory.total,fan.speed", "--format=csv")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
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
