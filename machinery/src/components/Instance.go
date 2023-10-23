package components

// hvd
import (
	"encoding/json"
	"io"
	"os"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func OpenInstance(instance *models.Instance) error {
	// Configuration is stored into a json file, and there is only 1 agent.
	// Open instance config
	jsonFile, err := os.Open("./data/instance.json")

	if err != nil {
		log.Log.Info("Instance file is not found " + "./data/instance.json")
	} else {
		log.Log.Info("Successfully opened instance.json")
		byteValue, _ := io.ReadAll(jsonFile)
		err = json.Unmarshal(byteValue, &instance)
		jsonFile.Close()

		if err != nil {
			log.Log.Error("JSON file not valid: " + err.Error())
		}
	}

	return err
}

func StoreInstance(resp models.LiveRestreamResp) error {
	// Save into database
	var instance models.Instance
	err := OpenInstance(&instance)
	// update
	if err != nil {
		instance.Name = os.Getenv("DEPLOYMENT_NAME")
		instance.LiveRestreams = make(map[string]models.LiveRestreamResp)
	}
	instance.LiveRestreams[resp.Stream] = resp

	res, _ := json.MarshalIndent(instance, "", "\t")
	err = os.WriteFile("./data/instance.json", res, 0644)

	if err != nil {
		log.Log.Warning("Can not write to ./data/instance.json: " + err.Error())
	}

	return err
}
