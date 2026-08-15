package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/domain"
	"github.com/Profy256/katafasoccerpredictions/backend/internal/postgres"
)

// revenueCmd reports where money came from over a window.
//
// Amounts print as whole shillings throughout. UGX has no minor unit, so a
// decimal point here would be an invented precision.
func revenueCmd(ctx context.Context, db *postgres.DB, args []string) error {
	fs := flag.NewFlagSet("revenue", flag.ContinueOnError)
	days := fs.Int("days", 30, "window length in days, ending today")
	fromFlag := fs.String("from", "", "window start, YYYY-MM-DD")
	toFlag := fs.String("to", "", "window end (inclusive), YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}

	to := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -*days)
	if *fromFlag != "" {
		parsed, err := time.Parse(time.DateOnly, *fromFlag)
		if err != nil {
			return fmt.Errorf("-from must be YYYY-MM-DD: %w", err)
		}
		from = parsed
	}
	if *toFlag != "" {
		parsed, err := time.Parse(time.DateOnly, *toFlag)
		if err != nil {
			return fmt.Errorf("-to must be YYYY-MM-DD: %w", err)
		}
		to = parsed.AddDate(0, 0, 1)
	}

	report, err := db.Revenue(ctx, from, to)
	if err != nil {
		return err
	}

	fmt.Printf("Revenue %s to %s\n\n",
		from.Format(time.DateOnly), to.AddDate(0, 0, -1).Format(time.DateOnly))

	fmt.Printf("  gross     UGX %12d  (%d purchases)\n", int64(report.GrossUGX), report.PaidPurchases)
	fmt.Printf("  refunded  UGX %12d  (%d purchases)\n", int64(report.RefundedUGX), report.RefundedPurchases)
	fmt.Printf("  net       UGX %12d\n", int64(report.NetUGX))
	fmt.Printf("  pending   UGX %12d  (%d awaiting the gateway)\n",
		int64(report.PendingUGX), report.PendingPurchases)
	fmt.Printf("  failed                     %d prompts never completed\n\n", report.FailedPurchases)

	printBuckets("By package", report.ByPackage)
	printBuckets("By analyst", report.ByAnalyst)
	printBuckets("By mobile provider", report.ByProvider)
	printBuckets("By day", report.ByDay)
	printBuckets("Top slips", report.BySlip)

	if report.PendingPurchases > 0 {
		// Worth saying out loud: pending money is money the gateway has not
		// resolved, and a number that stays high means reconciliation is not
		// keeping up rather than that sales are strong.
		fmt.Printf("\n%d purchase(s) still pending. Run `katafa settle` targets or check\n"+
			"reconcile_payments if this does not fall.\n", report.PendingPurchases)
	}
	return nil
}

func printBuckets(title string, buckets []postgres.RevenueBucket) {
	if len(buckets) == 0 {
		return
	}
	fmt.Println(title)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, b := range buckets {
		fmt.Fprintf(w, "  %s\tUGX %d\t%d purchase(s)\n", b.Label, int64(b.GrossUGX), b.Purchases)
	}
	_ = w.Flush()
	fmt.Println()
}

// paymentCmd resolves one trace code — the KTF-XXXXXXXX that appears on a
// MarzPay statement and in the payer's SMS — to the slip and buyer behind it.
func paymentCmd(ctx context.Context, db *postgres.DB, args []string) error {
	if len(args) == 0 {
		return errors.New("payment needs a trace code, e.g. katafa payment KTF-3F9A2B7C")
	}

	trace, err := db.PaymentByTraceCode(ctx, args[0])
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("no payment with trace code %q", args[0])
		}
		return err
	}

	fmt.Printf("%s\n\n", trace.TraceCode)
	fmt.Printf("  amount          UGX %d\n", int64(trace.AmountUGX))
	fmt.Printf("  transaction     %s\n", trace.Status)
	fmt.Printf("  purchase        %s (%s)\n", trace.PurchaseID, trace.PurchaseStatus)
	fmt.Printf("  slip            %s\n", trace.SlipTitle)
	fmt.Printf("  package         %s\n", trace.PackageCode)
	fmt.Printf("  buyer           %s <%s>\n", trace.UserName, trace.UserEmail)
	fmt.Printf("  paid from       %s", trace.PhoneNumber)
	if trace.MobileProvider != nil {
		fmt.Printf(" (%s)", *trace.MobileProvider)
	}
	fmt.Println()
	fmt.Printf("  reference       %s\n", trace.Reference)
	if trace.ProviderTxnID != nil {
		fmt.Printf("  marzpay txn     %s\n", *trace.ProviderTxnID)
	}
	if trace.ProviderUUID != nil {
		fmt.Printf("  marzpay uuid    %s\n", *trace.ProviderUUID)
	}
	fmt.Printf("  requested at    %s\n", trace.CreatedAt.UTC().Format(time.RFC3339))
	if trace.PaidAt != nil {
		fmt.Printf("  paid at         %s\n", trace.PaidAt.UTC().Format(time.RFC3339))
	}
	return nil
}
