package proc

import "os/exec"

// probeGroup kills the probe alone, as CommandContext does: a job, as
// StartTree gives a command, is joined only once it has started, and Output
// starts it out of reach. EndProbes still ends it when magpie exits.
func probeGroup(cmd *exec.Cmd, p *probe) {
	cmd.Cancel = func() error {
		defer p.end()
		return cmd.Process.Kill()
	}
}
