package provider

import "testing"

func TestReplacesMembers(t *testing.T) {
	parse := func(raw string) Document {
		document, err := ParseDocument(t.Context(), []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return document
	}
	one := parse(`{"outbounds":[{"type":"socks","tag":"one","server":"127.0.0.1","server_port":1080}]}`)
	changed := parse(`{"outbounds":[{"type":"socks","tag":"one","server":"127.0.0.2","server_port":1080}]}`)
	two := parse(`{"outbounds":[{"type":"socks","tag":"two","server":"127.0.0.1","server_port":1080}]}`)
	if ReplacesMembers(one, one) || ReplacesMembers(one, two) || ReplacesMembers(one, Document{}) {
		t.Fatal("unchanged or distinct members can use native membership acknowledgement")
	}
	if !ReplacesMembers(one, changed) {
		t.Fatal("same-tag server changes cannot be acknowledged by tag equality")
	}
}
