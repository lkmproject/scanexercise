package jobs

import "testing"

func TestJobPK(t *testing.T) {
	got := jobPK("abc")
	want := "PROJECT#scanexercise#JOB#abc"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestS3Key(t *testing.T) {
	if got := s3Key("abc", "input/0.jpg"); got != "jobs/abc/input/0.jpg" {
		t.Fatalf("got %s", got)
	}
}
