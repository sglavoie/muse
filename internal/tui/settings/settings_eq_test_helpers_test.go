package settings

func switchToEQTab(o SettingsOverlay) SettingsOverlay {
	o.Tab = TabEqualizer
	o.cursor = 0
	return o
}
