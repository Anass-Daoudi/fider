package middlewares

import (
	"time"

	"github.com/getfider/fider/app/pkg/dbx"

	"github.com/getfider/fider/app"
	"github.com/getfider/fider/app/models/dto"
	"github.com/getfider/fider/app/pkg/errors"
	"github.com/getfider/fider/app/pkg/log"
	"github.com/getfider/fider/app/pkg/web"
	webutil "github.com/getfider/fider/app/pkg/web/util"
	"github.com/getfider/fider/app/pkg/worker"
	"github.com/getfider/fider/app/storage/postgres"
)

//WorkerSetup current context with some services
func WorkerSetup() worker.MiddlewareFunc {
	return func(next worker.Job) worker.Job {
		return func(c *worker.Context) (err error) {
			start := time.Now()
			log.Debugf(c, "Task '@{TaskName:magenta}' started on worker '@{WorkerID:magenta}'", dto.Props{
				"TaskName": c.TaskName(),
				"WorkerID": c.WorkerID(),
			})

			trx, err := dbx.BeginTx(c)
			if err != nil {
				err = c.Failure(err)
				log.Debugf(c, "Task '@{TaskName:magenta}' finished in @{ElapsedMs:magenta}ms", dto.Props{
					"TaskName":  c.TaskName(),
					"ElapsedMs": time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			//In case it panics somewhere
			defer func() {
				if r := recover(); r != nil {
					c.Failure(errors.Panicked(r))
					trx.Rollback()
					log.Debugf(c, "Task '@{TaskName:magenta}' panicked in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
						"TaskName":  c.TaskName(),
						"ElapsedMs": time.Since(start).Nanoseconds() / int64(time.Millisecond),
					})
				}
			}()

			c.Set(app.TransactionCtxKey, trx)
			c.SetServices(&app.Services{
				Context: c,
				Tenants: postgres.NewTenantStorage(trx, c),
				Users:   postgres.NewUserStorage(trx, c),
				Posts:   postgres.NewPostStorage(trx, c),
			})

			//Execute the chain
			if err = next(c); err != nil {
				trx.Rollback()
				log.Debugf(c, "Task '@{TaskName:magenta}' finished in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
					"TaskName":  c.TaskName(),
					"ElapsedMs": time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			//No errors, so try to commit it
			if err = trx.Commit(); err != nil {
				log.Errorf(c, "Failed to commit request with: @{Error}", dto.Props{
					"Error": err.Error(),
				})
				log.Debugf(c, "Task '@{TaskName:magenta}' finished in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
					"TaskName":  c.TaskName(),
					"ElapsedMs": time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			//Still no errors, everything is fine!
			log.Debugf(c, "Task '@{TaskName:magenta}' finished in @{ElapsedMs:magenta}ms (committed)", dto.Props{
				"TaskName":  c.TaskName(),
				"ElapsedMs": time.Since(start).Nanoseconds() / int64(time.Millisecond),
			})
			return nil
		}
	}
}

//WebSetup current context with some services
func WebSetup() web.MiddlewareFunc {
	return func(next web.HandlerFunc) web.HandlerFunc {
		return func(c *web.Context) error {
			start := time.Now()
			log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} started for @{ClientIP:magenta}", dto.Props{
				"HttpMethod": c.Request.Method,
				"URL":        c.Request.URL.String(),
				"ClientIP":   c.Request.ClientIP,
			})

			//In case it panics somewhere
			defer func() {
				if r := recover(); r != nil {
					c.Failure(errors.Panicked(r))
					c.Rollback()
					log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} panicked in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
						"HttpMethod": c.Request.Method,
						"URL":        c.Request.URL.String(),
						"ElapsedMs":  time.Since(start).Nanoseconds() / int64(time.Millisecond),
					})
				}
			}()

			trx, err := dbx.BeginTx(c)
			if err != nil {
				err = c.Failure(err)
				log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} finished in @{ElapsedMs:magenta}ms", dto.Props{
					"HttpMethod": c.Request.Method,
					"URL":        c.Request.URL.String(),
					"ElapsedMs":  time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			oauthBaseURL := webutil.GetOAuthBaseURL(c)
			tenantStorage := postgres.NewTenantStorage(trx, c)

			c.Set(app.TransactionCtxKey, trx)
			c.SetServices(&app.Services{
				Context: c,
				Tenants: tenantStorage,
				OAuth:   web.NewOAuthService(oauthBaseURL, tenantStorage),
				Users:   postgres.NewUserStorage(trx, c),
				Posts:   postgres.NewPostStorage(trx, c),
			})

			//Execute the chain
			if err := next(c); err != nil {
				c.Rollback()
				log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} finished in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
					"HttpMethod": c.Request.Method,
					"URL":        c.Request.URL.String(),
					"ElapsedMs":  time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			//No errors, so try to commit it
			if err := c.Commit(); err != nil {
				log.Errorf(c, "Failed to commit request with: @{Error}", dto.Props{
					"Error": err.Error(),
				})
				log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} finished in @{ElapsedMs:magenta}ms (rolled back)", dto.Props{
					"HttpMethod": c.Request.Method,
					"URL":        c.Request.URL.String(),
					"ElapsedMs":  time.Since(start).Nanoseconds() / int64(time.Millisecond),
				})
				return err
			}

			//Still no errors, everything is fine!
			log.Infof(c, "@{HttpMethod:magenta} @{URL:magenta} finished in @{ElapsedMs:magenta}ms (committed)", dto.Props{
				"HttpMethod": c.Request.Method,
				"URL":        c.Request.URL.String(),
				"ElapsedMs":  time.Since(start).Nanoseconds() / int64(time.Millisecond),
			})
			return nil
		}
	}
}
