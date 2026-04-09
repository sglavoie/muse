package tui

func switchToEQTab(o SettingsOverlay) SettingsOverlay {
	o.tab = settingsTabEqualizer
	o.cursor = 0
	return o
}
