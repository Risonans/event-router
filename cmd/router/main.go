// Команда router прогоняет события из testdata/events.json через роутер
// и печатает статус каждого события и то, что ушло в MMP.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"eventrouter/internal/event"
	"eventrouter/internal/router"
	"eventrouter/internal/sender"
	"eventrouter/internal/store"
)

func main() {
	dir := flag.String("data", "testdata", "каталог с projects.json, mappings.json, consents.json, events.json")
	flag.Parse()

	st, err := store.Load(*dir)
	if err != nil {
		log.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(*dir, "events.json"))
	if err != nil {
		log.Fatal(err)
	}
	var events []event.Event
	if err := json.Unmarshal(raw, &events); err != nil {
		log.Fatal(err)
	}

	mmp := &sender.Fake{}
	r := router.New(st, mmp)

	fmt.Printf("%-8s %-16s %-8s %-18s %s\n", "EVENT", "TYPE", "PROJECT", "STATUS", "REASON")
	for _, ev := range events {
		res := r.Process(ev)
		fmt.Printf("%-8s %-16s %-8d %-18s %s\n", ev.ID, ev.Type, ev.ProjectID, res.Status, res.Reason)
	}

	fmt.Println("\nОтправлено в MMP:")
	for _, p := range mmp.Sent() {
		fmt.Printf("  %s  %-8s %10.2f %s  mmp_id=%s sandbox=%v\n", p.EventID, p.EventName, p.Amount, p.Currency, p.MMPID, p.Sandbox)
	}
}
