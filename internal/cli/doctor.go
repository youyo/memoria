package cli

import (
	"fmt"
	"io"
)

// DoctorCmd はシステム診断コマンドを定義する。
type DoctorCmd struct{}

// Run は doctor コマンドを実行する。
func (c *DoctorCmd) Run(globals *Globals, w *io.Writer) error {
	fmt.Fprintln(*w, "not implemented")
	return nil
}
