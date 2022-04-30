package nvidia

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
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
