package switchcmd

type cli struct {
	*switchGit
}

func newCli(sg *switchGit) *cli {
	return &cli{switchGit: sg}
}

func (s *cli) run(target string, force bool) {
	results := s.performSwitch(target, force)
	printRichResults(s.repos, results)
}
