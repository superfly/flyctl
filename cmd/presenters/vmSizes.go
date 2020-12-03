package presenters

import (
	"fmt"

	"github.com/superfly/flyctl/api"
)

type VMSizes struct {
	VMSizes []api.VMSize
}

func (p *VMSizes) APIStruct() interface{} {
	return p.VMSizes
}

func (p *VMSizes) FieldNames() []string {
	return []string{"Name", "CPU Cores", "Memory"}
}

func (p *VMSizes) Records() []map[string]string {
	out := []map[string]string{}

	for _, size := range p.VMSizes {
		out = append(out, map[string]string{
			"Name":      size.Name,
			"CPU Cores": formatCores(size),
			"Memory":    formatMemory(size),
		})
	}

	return out
}

func formatCores(size api.VMSize) string {
	if size.CPUCores < 1.0 {
		return fmt.Sprintf("%.2f", size.CPUCores)
	}
	return fmt.Sprintf("%d", int(size.CPUCores))
}

func formatMemory(size api.VMSize) string {
	if size.MemoryGB < 1.0 {
		return fmt.Sprintf("%d MB", size.MemoryMB)
	}
	return fmt.Sprintf("%d GB", int(size.MemoryGB))
}
