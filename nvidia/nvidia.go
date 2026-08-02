package nvidia

import (
	"bytes"
	"log"
	"os/exec"
	"strings"
)

// GetInformations retrieves GPU information using nvidia-smi command
func GetInformations() (string, map[string]string) {
	log.Println("Retrieving NVIDIA GPU information")

	// Check if nvidia-smi is available
	probeCmd := exec.Command("which", "nvidia-smi")
	var probeOut bytes.Buffer
	probeCmd.Stdout = &probeOut
	err := probeCmd.Run()
	if err != nil {
		log.Printf("Error probing for nvidia-smi: %v", err)
		return "", nil
	}

	if probeOut.String() == "" {
		log.Println("nvidia-smi not found")
		return "", nil
	}

	// Execute nvidia-smi with query parameters
	cmd := exec.Command(
		"nvidia-smi",
		"--query-gpu=name,temperature.gpu,memory.used,utilization.gpu,power.draw,clocks.current.graphics,clocks.current.memory,power.limit,power.max_limit,temperature.memory,memory.total,fan.speed",
		"--format=csv",
	)
	var out bytes.Buffer
	cmd.Stdout = &out

	err = cmd.Run()
	if err != nil {
		log.Fatalf("Error executing nvidia-smi: %v", err)
		return "", nil
	}

	// Parse the output
	output := out.String()
	log.Printf("Raw GPU data:\n%s", output)

	data := make(map[string]string)
	lines := strings.Split(output, "\n")

	// Skip header line and process first GPU entry
	if len(lines) < 2 {
		return "", nil
	}

	fields := strings.Split(lines[1], ",")
	if len(fields) != 12 {
		log.Println("Unexpected number of fields in nvidia-smi output")
		return "", nil
	}

	// Map the fields to more descriptive keys
	data["gpu.name"] = fields[0]
	data["gpu.temperature"] = fields[1]
	data["memory.used"] = fields[2]
	data["gpu.utilization"] = fields[3]
	data["power.draw"] = fields[4]
	data["clocks.graphics"] = fields[5]
	data["clocks.memory"] = fields[6]
	data["power.limit"] = fields[7]
	data["power.max_limit"] = fields[8]
	data["memory.temperature"] = fields[9]
	data["memory.total"] = fields[10]
	data["fan.speed"] = fields[11]

	return data["gpu.name"], data
}
