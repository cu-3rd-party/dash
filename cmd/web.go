package cmd

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.cu3rd.ru/cu3rd/dash/models/db"
	member_model "git.cu3rd.ru/cu3rd/dash/models/member"
	"git.cu3rd.ru/cu3rd/dash/modules/setting"
	"git.cu3rd.ru/cu3rd/dash/routers"
	cloudtips_service "git.cu3rd.ru/cu3rd/dash/services/cloudtips"
	finance_service "git.cu3rd.ru/cu3rd/dash/services/finance"
	rates_service "git.cu3rd.ru/cu3rd/dash/services/rates"

	"github.com/urfave/cli/v3"
)

const (
	shutdownTimeout = 10 * time.Second
	ratesInterval   = 24 * time.Hour
	// the feed is small, but the Bank of Russia sits behind DDoS protection: retry gently
	ratesRetry = time.Hour
)

// updateRatesDaily loads the rates at start and then once a day; a failure retries in an hour.
func updateRatesDaily(ctx context.Context) {
	for {
		wait := ratesInterval
		if date, err := rates_service.Update(ctx, "system"); err != nil {
			slog.Error("update rates", "err", err)
			wait = ratesRetry
		} else {
			slog.Info("rates updated", "date", date)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// syncCloudTips imports CloudTips donations at start and then every DASH_CLOUDTIPS_INTERVAL.
func syncCloudTips(ctx context.Context) {
	for {
		if n, err := cloudtips_service.Sync(ctx); err != nil {
			slog.Error("sync cloudtips", "err", err)
		} else if n > 0 {
			slog.Info("cloudtips donations imported", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(setting.CloudTipsInterval):
		}
	}
}

func cmdWeb() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "Start the web server",
		Action: func(parent context.Context, _ *cli.Command) error {
			slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
			setting.Load()
			if err := setting.Validate(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			if err := db.InitEngine(ctx); err != nil {
				return err
			}
			for _, login := range setting.BootstrapOwners {
				if err := member_model.Upsert(ctx, &member_model.Member{Login: login, Role: member_model.RoleOwner, CreatedBy: "bootstrap"}); err != nil {
					return err
				}
			}
			if !setting.OAuthEnabled() && !setting.DevLogin && !setting.PasswordLoginEnabled() {
				// the public page still works; the admin pages wait for the OAuth app
				slog.Warn("sign-in is not configured: set DASH_OAUTH_CLIENT_ID and DASH_OAUTH_CLIENT_SECRET")
			}
			if setting.PasswordLoginEnabled() {
				// the password login is a member like any other; make sure it can get in
				if _, err := member_model.GetByLogin(ctx, setting.PasswordLogin); db.IsErrNotExist(err) {
					if err := member_model.Upsert(ctx, &member_model.Member{Login: setting.PasswordLogin, Role: member_model.RoleOwner, CreatedBy: "password-login"}); err != nil {
						return err
					}
				}
			}
			if setting.DevLogin {
				slog.Warn("DASH_DEV_LOGIN is on: anyone can sign in as any member, local runs only")
			}
			if setting.SeedDemo {
				if err := finance_service.SeedDemo(ctx); err != nil {
					return err
				}
			}

			if setting.RatesURL != "" {
				go updateRatesDaily(ctx)
			}

			if cloudtips_service.Enabled() {
				go syncCloudTips(ctx)
			}

			slog.Info("listening", "addr", setting.HTTPAddr)
			srv := &http.Server{
				Addr:              setting.HTTPAddr,
				Handler:           routers.NormalRoutes(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				defer cancel()
				_ = srv.Shutdown(shutdownCtx)
			}()
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
}

func cmdMigrate() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Apply database migrations and exit",
		Action: func(ctx context.Context, _ *cli.Command) error {
			setting.Load()
			return db.InitEngine(ctx)
		},
	}
}
