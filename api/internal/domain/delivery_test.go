package domain

import "testing"

func TestDeliveryStatusIsTerminal(t *testing.T) {
	if DeliveryPending.IsTerminal() {
		t.Fatal("pending is not terminal")
	}
	if !DeliverySucceeded.IsTerminal() {
		t.Fatal("succeeded is terminal")
	}
	if !DeliveryDeadLetter.IsTerminal() {
		t.Fatal("dead letter is terminal")
	}
}
