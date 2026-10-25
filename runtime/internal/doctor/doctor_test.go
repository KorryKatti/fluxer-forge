package doctor

import (
	"testing"

	"github.com/KorryKatti/fluxer-forge/runtime/internal/config"
)

func TestChannelIDsListsEachChannelOnce(t *testing.T) {
	cfg := &config.Config{}
	cfg.Modules.Welcome.Enabled = true
	cfg.Modules.Welcome.OnJoin.ChannelID = "c1"
	cfg.Modules.Welcome.OnLeave.Enabled = true
	cfg.Modules.Welcome.OnLeave.ChannelID = "c1"
	cfg.Modules.ReactionRoles.Rules = []config.ReactionRoleRule{
		{ChannelID: "c2"},
		{ChannelID: "c1"},
	}

	got := channelIDs(cfg)
	if len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Errorf("channelIDs = %v, want [c1 c2]", got)
	}
}
