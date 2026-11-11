package httpapi

import (
	json "encoding/json/v2"
	"net/http"
	"testing"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
)

func TestResourcesRegionSelectionWhileStopped(t *testing.T) {
	server := resourceFixture(t)
	response, _ := resourceRequest(t, server, "PUT", "/api/v1/selection", `{"group_id":"default","node_tag":"","region":"other"}`)
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	config, err := moduleconfig.LoadModule(server.options.ModuleConfig)
	if err != nil || config.Region != "other" {
		t.Fatalf("region not saved: %+v %v", config, err)
	}
	response, result := resourceRequest(t, server, "GET", "/api/v1/selection", "")
	var selection service.Selection
	if err := json.Unmarshal(result.Data, &selection); err != nil || response.Code != http.StatusOK || selection.ActiveRegion != "other" {
		t.Fatalf("region not returned: %s %v", response.Body.String(), err)
	}
	response, _ = resourceRequest(t, server, "PUT", "/api/v1/selection", `{"group_id":"default","node_tag":"","region":"jp"}`)
	if response.Code == http.StatusOK {
		t.Fatal("accepted a region without nodes")
	}
}
