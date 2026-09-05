package dispatch

import (
	"context"
	"github.com/gabinante/flywheel/internal/runstatus"
)

type liveSessionParser struct {
	OutputParser
	ctx context.Context
}

func (p *liveSessionParser) ParseLine(line string) ([]ParsedEvent, bool) {
	runstatus.ParseOutput(p.ctx, []byte(line))
	return p.OutputParser.ParseLine(line)
}
