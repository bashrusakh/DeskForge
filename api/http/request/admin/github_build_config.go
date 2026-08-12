package admin

// GithubBuildConfigForm is the complete user-editable GitHub build contract.
// Workflow names and refs are application/provider-owned values, not request
// fields. Empty secrets preserve the currently stored value.
type GithubBuildConfigForm struct {
	Repo       string `json:"repo"`
	Token      string `json:"token"`
	PayloadKey string `json:"payload_key"`
}

// WorkflowRefApprovalForm is separate from the normal settings form. Clients
// submit only the label selected from the provider-derived tag options; raw
// refs and SHAs are not accepted as normal input.
type WorkflowRefApprovalForm struct {
	WorkflowTag string `json:"workflow_tag"`
	Confirm     bool   `json:"confirm"`
}
