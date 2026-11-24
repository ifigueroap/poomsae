package daemon

func registerProjectOperationPolicies(policies map[string]HostOperationPolicy) {
	registerHostOperations(policies, HostOperationPolicy{
		Kind: hostOperationProjectRead, Capability: hostCapabilityRead,
	}, "listProjects", "showProject", "readProjectByName")

	registerHostOperations(policies, HostOperationPolicy{
		Kind: hostOperationProjectAdministration, Capability: hostCapabilityManage,
		Mutation: true, restricted: true,
	}, "resolveProject", "initProject", "mergeProject", "removeProject", "purgeProject",
		"restoreProject", "detachProjectAlias", "renameProject", "patchProjectMetadata")

	registerHostOperations(policies, HostOperationPolicy{
		Kind: hostOperationIntegrationAdministration, Capability: hostCapabilityManage,
		restricted: true,
	}, "doctor")
}
