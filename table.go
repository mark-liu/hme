package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

// WriteTable writes a formatted table of HME emails to w.
func WriteTable(w io.Writer, emails []HmeEmail) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tLABEL\tACTIVE\tCREATED\tFORWARD TO")
	for _, e := range emails {
		created := ""
		if e.CreateTimestamp > 0 {
			created = time.UnixMilli(e.CreateTimestamp).Format("2006-01-02")
		}
		active := "yes"
		if !e.IsActive {
			active = "no"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			e.Hme, e.Label, active, created, e.ForwardToEmail)
	}
	tw.Flush()
}
