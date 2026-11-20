package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// WarnRate is the rate below which a run asks for confirmation.
const WarnRate = 1.0

// ConfirmLowRate warns about a rate below WarnRate and, on a terminal, asks to continue.
// Rates below domain.MinRate are refused earlier, by domain.Config.Validate.
func ConfirmLowRate(w io.Writer, rate float64) error {
	if rate < WarnRate {
		fmt.Fprintf(w, "\nWARNING: Low rate limit detected (%.2f req/s)\n", rate)
		fmt.Fprintf(w, "This will send requests very slowly:\n")
		fmt.Fprintf(w, "- %.2f requests per second\n", rate)
		fmt.Fprintf(w, "- ~%.0f requests per minute\n", rate*60)
		fmt.Fprintf(w, "- Test may take a long time to complete\n\n")

		// If interactive terminal, prompt for confirmation
		if IsInteractiveTerminal() {
			fmt.Fprintf(w, "Do you want to continue with this rate? (y/N): ")
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading confirmation: %w", err)
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				return fmt.Errorf("test canceled by user")
			}
			fmt.Fprintf(w, "\n")
		} else {
			fmt.Fprintf(w, "Continuing in non-interactive mode...\n\n")
		}
	}

	return nil
}

// IsInteractiveTerminal checks if the program is running in an interactive terminal.
func IsInteractiveTerminal() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	// Check if stdin is a character device (terminal) rather than a pipe or file
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
