package store

import (
	"runtime"
	"testing"
)

func TestSummaryFilePath(t *testing.T) {
	tests := []struct {
		name        string
		sessionPath string
		want        string
	}{
		{
			name:        "normal path with .json extension",
			sessionPath: "/data/sessions/12345.json",
			want:        "/data/sessions/12345_summary.json",
		},
		{
			name:        "normal path with .txt extension",
			sessionPath: "/data/sessions/session.txt",
			want:        "/data/sessions/session_summary.json",
		},
		{
			name:        "no extension",
			sessionPath: "/data/sessions/session",
			want:        "/data/sessions/session_summary.json",
		},
		{
			name:        "single filename with extension",
			sessionPath: "session.json",
			want:        "session_summary.json",
		},
		{
			name:        "single filename without extension",
			sessionPath: "session",
			want:        "session_summary.json",
		},
		{
			name:        "relative path with extension",
			sessionPath: "./sessions/session.json",
			want:        "./sessions/session_summary.json",
		},
		{
			name:        "absolute path with extension",
			sessionPath: "/absolute/path/to/session.json",
			want:        "/absolute/path/to/session_summary.json",
		},
		{
			name:        "path with multiple dots",
			sessionPath: "/data/sessions/my.session.file.json",
			want:        "/data/sessions/my.session.file_summary.json",
		},
		{
			name:        "path with directory dots",
			sessionPath: "/data/../sessions/session.json",
			want:        "/data/../sessions/session_summary.json",
		},
		{
			name:        "empty string",
			sessionPath: "",
			want:        "_summary.json",
		},
		{
			name:        "only extension",
			sessionPath: ".json",
			want:        "_summary.json",
		},
		{
			name:        "deep nested path",
			sessionPath: "/a/b/c/d/e/f/session.json",
			want:        "/a/b/c/d/e/f/session_summary.json",
		},
		{
			name:        "path with trailing slash (no file)",
			sessionPath: "/data/sessions/",
			want:        "/data/sessions/_summary.json",
		},
		{
			name:        "path with special characters",
			sessionPath: "/data/sessions/session-v1.2.3.json",
			want:        "/data/sessions/session-v1.2.3_summary.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummaryFilePath(tt.sessionPath)
			if got != tt.want {
				t.Errorf("SummaryFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSummaryFilePath_WindowsPaths tests Windows-specific paths
func TestSummaryFilePath_WindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific tests on non-Windows OS")
	}

	tests := []struct {
		name        string
		sessionPath string
		want        string
	}{
		{
			name:        "Windows absolute path",
			sessionPath: `C:\data\sessions\session.json`,
			want:        `C:\data\sessions\session_summary.json`,
		},
		{
			name:        "Windows relative path",
			sessionPath: `.\sessions\session.json`,
			want:        `.\sessions\session_summary.json`,
		},
		{
			name:        "Windows UNC path",
			sessionPath: `\\server\share\sessions\session.json`,
			want:        `\\server\share\sessions\session_summary.json`,
		},
		{
			name:        "Windows path with forward slash",
			sessionPath: `C:/data/sessions/session.json`,
			want:        `C:/data/sessions/session_summary.json`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummaryFilePath(tt.sessionPath)
			if got != tt.want {
				t.Errorf("SummaryFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSummaryFilePath_UnixPaths tests Unix/Linux specific paths
func TestSummaryFilePath_UnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific tests on Windows OS")
	}

	tests := []struct {
		name        string
		sessionPath string
		want        string
	}{
		{
			name:        "home directory path",
			sessionPath: "~/data/sessions/session.json",
			want:        "~/data/sessions/session_summary.json",
		},
		{
			name:        "root path",
			sessionPath: "/session.json",
			want:        "/session_summary.json",
		},
		{
			name:        "hidden file",
			sessionPath: "/data/sessions/.session.json",
			want:        "/data/sessions/.session_summary.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummaryFilePath(tt.sessionPath)
			if got != tt.want {
				t.Errorf("SummaryFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSummaryFilePath_EdgeCases tests various edge cases
func TestSummaryFilePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		sessionPath string
		want        string
	}{
		{
			name:        "path with only filename and no dir",
			sessionPath: "a.json",
			want:        "a_summary.json",
		},
		{
			name:        "path with single character filename",
			sessionPath: "x.json",
			want:        "x_summary.json",
		},
		{
			name:        "path with underscore already",
			sessionPath: "/data/sessions/my_session.json",
			want:        "/data/sessions/my_session_summary.json",
		},
		{
			name:        "path with space",
			sessionPath: "/data/sessions/my session.json",
			want:        "/data/sessions/my session_summary.json",
		},
		{
			name:        "path with uppercase extension",
			sessionPath: "/data/sessions/session.JSON",
			want:        "/data/sessions/session_summary.json",
		},
		{
			name:        "path with mixed case extension",
			sessionPath: "/data/sessions/session.JsOn",
			want:        "/data/sessions/session_summary.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummaryFilePath(tt.sessionPath)
			if got != tt.want {
				t.Errorf("SummaryFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSummaryFilePath_Consistency tests that the function is deterministic
func TestSummaryFilePath_Consistency(t *testing.T) {
	testPath := "/data/sessions/session.json"
	
	// Call the function multiple times with the same input
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		results[i] = SummaryFilePath(testPath)
	}
	
	// All results should be identical
	expected := SummaryFilePath(testPath)
	for i, result := range results {
		if result != expected {
			t.Errorf("Call %d: SummaryFilePath() = %v, want %v", i, result, expected)
		}
	}
}