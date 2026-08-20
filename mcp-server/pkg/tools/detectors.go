package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	processor_api "github.com/runtime-radar/runtime-radar/event-processor/api"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
)

const (
	// detectorPageSize is how many detectors are read per Event Processor page.
	detectorPageSize = 100
	// maxDetectorPages bounds the paging loop; the product ships a few dozen
	// detectors, so this is a runaway guard rather than a real limit.
	maxDetectorPages = 20
	// detectorOrder sorts the listing by name, the way the UI shows it.
	detectorOrder = "name asc"
)

// mitreRE matches MITRE ATT&CK technique and tactic identifiers (T1055,
// T1055.001, TA0002) mentioned in a detector's description. The product does
// not carry ATT&CK mapping as a field, so what a detector's author wrote is all
// there is.
var mitreRE = regexp.MustCompile(`\b(?:T\d{4}(?:\.\d{3})?|TA\d{4})\b`)

// ListDetectorsArgs are the arguments of list_detectors.
type ListDetectorsArgs struct {
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring to filter detectors by identifier, name or description"`
}

// DetectorInfo describes one threat detector of Event Processor.
type DetectorInfo struct {
	ID          string   `json:"id" jsonschema:"identifier of the detector, for example CS_RT_CRYPTOMINER. Pass it to search_runtime_events as detector_ids"`
	Name        string   `json:"name,omitempty" jsonschema:"human readable name of the detector"`
	Description string   `json:"description,omitempty" jsonschema:"what the detector looks for"`
	Version     uint32   `json:"version,omitempty" jsonschema:"version of the detector"`
	Author      string   `json:"author,omitempty" jsonschema:"author of the detector"`
	License     string   `json:"license,omitempty" jsonschema:"license of the detector"`
	MitreIDs    []string `json:"mitre_ids,omitempty" jsonschema:"MITRE ATT&CK technique or tactic identifiers mentioned in the description, when its author put any there"`
}

// ListDetectorsResult is the answer of list_detectors.
type ListDetectorsResult struct {
	Detectors []DetectorInfo `json:"detectors" jsonschema:"detectors installed in Event Processor, by name"`
	Count     int            `json:"count" jsonschema:"number of detectors returned"`
	Total     int            `json:"total" jsonschema:"number of detectors installed, before the search filter was applied"`
	Note      string         `json:"note" jsonschema:"how to read the fields this listing cannot fill in"`
}

// detectorSeverityNote explains an absence a caller would otherwise take for a
// bug: severity is decided per detection, not per detector, so there is no
// severity to list here.
const detectorSeverityNote = "Severity is not a property of a detector: it is assigned per detection, and is " +
	"returned with every threat in search_runtime_events and get_runtime_event. MITRE ATT&CK identifiers are " +
	"listed only when the detector's author mentioned them in its description."

func registerDetectorTools(server *mcp.Server, deps *Deps) {
	addTool(server, deps, &mcp.Tool{
		Name:        "list_detectors",
		Annotations: readOnly("List detectors"),
		Description: "List the threat detectors installed in Event Processor: identifier, name, description, " +
			"version, author and any MITRE ATT&CK identifiers their descriptions mention. Use it to learn which " +
			"detector identifiers exist before filtering events by detector_ids, and to understand what a threat " +
			"reported on an event actually means.",
	}, []auth.Permission{auth.ReadSystemSettings()}, listDetectors(deps))
}

func listDetectors(deps *Deps) func(context.Context, ListDetectorsArgs) (ListDetectorsResult, error) {
	return func(ctx context.Context, args ListDetectorsArgs) (ListDetectorsResult, error) {
		var (
			detectors []*processor_api.Detector
			total     int
		)

		for page := uint32(0); page < maxDetectorPages; page++ {
			resp, err := deps.Clients.Detectors.ListPage(ctx, &processor_api.ListDetectorPageReq{
				PageNum:  page,
				PageSize: detectorPageSize,
				Order:    detectorOrder,
			})
			if err != nil {
				return ListDetectorsResult{}, fmt.Errorf("can't list detectors: %w", err)
			}

			total = int(resp.GetTotal())
			detectors = append(detectors, resp.GetDetectors()...)

			if len(resp.GetDetectors()) < detectorPageSize || len(detectors) >= total {
				break
			}
		}

		search := strings.ToLower(strings.TrimSpace(args.Search))

		result := ListDetectorsResult{
			Detectors: make([]DetectorInfo, 0, len(detectors)),
			Total:     total,
			Note:      detectorSeverityNote,
		}

		for _, detector := range detectors {
			info := DetectorInfo{
				ID:          detector.GetId(),
				Name:        detector.GetName(),
				Description: detector.GetDescription(),
				Version:     detector.GetVersion(),
				Author:      detector.GetAuthor(),
				License:     detector.GetLicense(),
				MitreIDs:    mitreIDs(detector.GetDescription(), detector.GetName()),
			}

			if search != "" && !info.matches(search) {
				continue
			}

			result.Detectors = append(result.Detectors, info)
		}

		result.Count = len(result.Detectors)

		return result, nil
	}
}

func (d DetectorInfo) matches(loweredSearch string) bool {
	return strings.Contains(strings.ToLower(d.ID), loweredSearch) ||
		strings.Contains(strings.ToLower(d.Name), loweredSearch) ||
		strings.Contains(strings.ToLower(d.Description), loweredSearch)
}

// mitreIDs collects the ATT&CK identifiers mentioned in a detector's metadata,
// deduplicated and sorted.
func mitreIDs(parts ...string) []string {
	unique := make(map[string]struct{})

	for _, part := range parts {
		for _, id := range mitreRE.FindAllString(part, -1) {
			unique[id] = struct{}{}
		}
	}

	if len(unique) == 0 {
		return nil
	}

	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}
