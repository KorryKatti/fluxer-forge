package template

import "testing"

func TestRenderAllTokens(t *testing.T) {
	got := Render(
		"{user} {user.name} {user.id} {guild} {guild.id} {channel} {member.count}",
		Context{
			UserID:      "u1",
			UserName:    "ada",
			DisplayName: "Ada",
			GuildID:     "g1",
			GuildName:   "Test",
			ChannelID:   "c1",
			ChannelName: "general",
			MemberCount: 42,
		},
	)
	want := "<@u1> Ada u1 Test g1 general 42"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestRenderLeavesUnknownTokensAlone(t *testing.T) {
	got := Render("hello {nope}", Context{UserID: "u1"})
	if got != "hello {nope}" {
		t.Errorf("Render = %q", got)
	}
}

func TestRenderFallsBackForMissingValues(t *testing.T) {
	got := Render("{user} {guild} {channel}", Context{UserID: "u1", ChannelID: "c1"})
	want := "<@u1> this server c1"
	if got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}
