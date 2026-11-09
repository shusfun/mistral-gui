package app

// Caller holds mu. An empty version supports settings saved before revision selection.
func (a *App) modelLocked(id, version string) (Installed, bool) {
	var selected Installed
	found := false
	for _, m := range a.installed {
		if m.Kind == "model" && m.ID == id && (version == "" || m.Version == version) {
			selected, found = m, true
		}
	}
	return selected, found
}

func (a *App) selectModelLocked(m Installed) {
	old, ok := a.modelLocked(a.config.Model, a.config.ModelVersion)
	if ok && old.Path != m.Path {
		a.config.PreviousModel, a.config.PreviousModelVersion = old.ID, old.Version
	}
	a.config.Model, a.config.ModelVersion = m.ID, m.Version
}
