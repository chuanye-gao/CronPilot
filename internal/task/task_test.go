package task

import "testing"

func TestDeliveryValidationAndNotificationPolicy(t *testing.T) {
	valid := Delivery{Type: "email", To: []string{"owner@example.com"}, On: []string{"failed", "timeout"}}
	if err := valid.Validate("task"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if valid.ShouldNotify("success") || !valid.ShouldNotify("failed") {
		t.Fatal("ShouldNotify() did not follow configured statuses")
	}

	invalid := Delivery{Type: "email", To: []string{"not-an-email"}, On: []string{"success"}}
	if err := invalid.Validate("task"); err == nil {
		t.Fatal("Validate() accepted an invalid email address")
	}
}

func TestDeliveryIncludesOutputDefaultsToTrue(t *testing.T) {
	if !(Delivery{}).IncludesOutput() {
		t.Fatal("IncludesOutput() = false, want true by default")
	}
	value := false
	if (Delivery{IncludeOutput: &value}).IncludesOutput() {
		t.Fatal("IncludesOutput() = true, want false")
	}
}
