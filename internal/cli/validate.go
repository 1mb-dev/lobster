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

// ConfirmLowRate warns about a rate below WarnRate and, when in is a terminal, asks to continue.
// Rates below domain.MinRate are refused earlier, by domain.Config.Validate.
func ConfirmLowRate(w io.Writer, in *os.File, rate float64) error {
	if rate < WarnRate {
		fmt.Fprintf(w, "\nWARNING: Low rate limit detected (%.2f req/s)\n", rate)
		fmt.Fprintf(w, "This will send requests very slowly:\n")
		fmt.Fprintf(w, "- %.2f requests per second\n", rate)
		fmt.Fprintf(w, "- ~%.0f requests per minute\n", rate*60)
		fmt.Fprintf(w, "- Test may take a long time to complete\n\n")

		// If interactive terminal, prompt for confirmation
		if isTerminal(in) {
			fmt.Fprintf(w, "Do you want to continue with this rate? (y/N): ")
			reader := bufio.NewReader(in)
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
