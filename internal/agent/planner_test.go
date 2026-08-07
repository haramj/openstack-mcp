package agent

import "testing"

func TestPlanCreateUsesMemoryDefaults(t *testing.T) {
	memory := DefaultMemory()

	plan := PlanInstanceOperation(PlanInput{
		Operation: "create",
		Name:      "dev-box",
	}, memory)

	if plan.Blocked {
		t.Fatalf("expected create plan to be allowed, got reasons: %v", plan.Reasons)
	}
	if plan.RecommendedTool != "create_instance" {
		t.Fatalf("expected create_instance tool, got %q", plan.RecommendedTool)
	}
	if plan.Arguments["image"] != memory.DefaultImage {
		t.Fatalf("expected default image %q, got %q", memory.DefaultImage, plan.Arguments["image"])
	}
	if plan.Arguments["flavor"] != memory.DefaultFlavor {
		t.Fatalf("expected default flavor %q, got %q", memory.DefaultFlavor, plan.Arguments["flavor"])
	}
	if plan.Arguments["network"] != memory.DefaultNetwork {
		t.Fatalf("expected default network %q, got %q", memory.DefaultNetwork, plan.Arguments["network"])
	}
}

func TestPlanDeleteRequiresConfirmation(t *testing.T) {
	plan := PlanInstanceOperation(PlanInput{
		Operation:   "delete",
		Name:        "dev-box",
		ConfirmName: "wrong",
	}, DefaultMemory())

	if !plan.Blocked {
		t.Fatal("expected delete plan to be blocked without exact confirmation")
	}
}

func TestPlanBlocksProtectedRiskyLifecycle(t *testing.T) {
	plan := PlanInstanceOperation(PlanInput{
		Operation:  "admin_instance_action",
		Name:       "prod-api",
		Action:     "reboot",
		RebootType: "hard",
	}, DefaultMemory())

	if !plan.Blocked {
		t.Fatal("expected hard reboot of protected instance to be blocked")
	}
}

func TestPlanAllowsProtectedRestorativeLifecycle(t *testing.T) {
	plan := PlanInstanceOperation(PlanInput{
		Operation: "admin_instance_action",
		Name:      "prod-api",
		Action:    "start",
	}, DefaultMemory())

	if plan.Blocked {
		t.Fatalf("expected start of protected instance to be allowed, got reasons: %v", plan.Reasons)
	}
}
