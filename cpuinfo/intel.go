package cpuinfo

import (
	"bytes"
	"encoding/json"
	"log"
	"os/exec"
)

func GetInformations() (string, map[string]string) {
	log.Println("intel infos")
	var result = make(map[string]string)

	cmd := exec.Command("sensors", "-j")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	str := out.Bytes()

	var data map[string]interface{}
	err = json.Unmarshal(str, &data)
	if err != nil {
		log.Fatal("Error during Unmarshal():", err)
	}
	for k, v := range data {
		println("key", k)
		println("val", v)
	}

	// if skylake, ok := data["pch_skylake-virtual-0"]; ok {
	// 	print(skylake)
	// 	// m := skylake.(map[string]interface{})
	// 	// tempEntry := m["temp1"].(map[string]interface{})
	// 	// result["cpu.temp"] = tempEntry["temp1_input"].(string)
	// }
	// if coretemp, ok := data["coretemp-isa-0000"]; ok {
	// 	coreTempEntry := coretemp.(map[string]interface{})
	// 	packageEntry := coreTempEntry["Package id 0"].(map[string]interface{})
	// 	result["cpu.package"] = packageEntry["temp1_input"].(string)

	// 	for key, val := range coreTempEntry {
	// 		if strings.Contains(key, "Core") {
	// 			coreEntry := val.(map[string]interface{})
	// 			for k, v := range coreEntry {
	// 				if strings.Contains(k, "input") {
	// 					result[key] = v.(string)
	// 				}
	// 			}
	// 		}
	// 	}
	// }
	log.Println("intel infos END")
	print(result)
	return "unknown", result
}
