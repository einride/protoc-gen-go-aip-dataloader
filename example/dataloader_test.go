package example

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	freightv1 "go.einride.tech/protoc-gen-go-aip-dataloader/example/internal/proto/gen/einride/example/freight/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestDataloader(t *testing.T) {
	t.Run("Load", func(t *testing.T) {
		t.Parallel()
		t.Run("Single parent", func(t *testing.T) {
			t.Run("SingleKey", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				gotSite, err := dl.Load("shippers/1", site1.Name)
				assert.NilError(t, err)

				// should receive correct site
				assert.DeepEqual(t, site1, gotSite, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 1, len(client.recvRequests))
				// should be correct request
				expectedRequest := &freightv1.BatchGetSitesRequest{
					Parent: "shippers/1",
					Names: []string{
						site1.Name,
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
			})

			t.Run("MissingKey", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				client := &mockSiteService{}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// try to load missing site
				gotSite, err := dl.Load("shippers/1", "shippers/1/site/999")
				assert.Assert(t, cmp.Nil(gotSite))
				assert.Equal(t, status.Code(err), codes.NotFound)

				// should only be one request
				assert.Equal(t, 1, len(client.recvRequests))
				// should be correct request
				expectedRequest := &freightv1.BatchGetSitesRequest{
					Parent: "shippers/1",
					Names: []string{
						"shippers/1/site/999",
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
			})

			t.Run("DuplicateKeys", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// load the same key twice (one by one)
				gotSite1, err := dl.Load("shippers/1", site1.Name)
				assert.NilError(t, err)
				gotSite2, err := dl.Load("shippers/1", site1.Name)
				assert.NilError(t, err)

				// should receive correct site (twice)
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site1, gotSite2, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 1, len(client.recvRequests))
				// should be correct request
				expectedRequest := &freightv1.BatchGetSitesRequest{
					Parent: "shippers/1",
					Names: []string{
						site1.Name,
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
			})

			t.Run("ExistingAndMissingKeys", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// try to load missing key
				gotSite1, err := dl.Load("shippers/1", "shippers/1/sites/999")
				assert.Assert(t, cmp.Nil(gotSite1))
				assert.Equal(t, status.Code(err), codes.NotFound)
				// load existing key
				gotSite2, err := dl.Load("shippers/1", site1.Name)
				assert.NilError(t, err)

				// should receive correct site
				assert.DeepEqual(t, site1, gotSite2, protocmp.Transform())
				// should only be two requests
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							"shippers/1/sites/999",
						},
					},
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
						},
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests, protocmp.Transform())
			})
			t.Run("AboveMaxBatchLimitConcurrently", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
				sites := []*freightv1.Site{
					site1,
					site2,
				}
				client := &mockSiteService{
					sites: sites,
				}
				const timeoutLimit = time.Millisecond * 100
				const batchLimit = 1
				dl := NewSitesDataloader(
					ctx,
					client,
					timeoutLimit,
					batchLimit,
				)

				// Start timer
				t1 := time.Now()
				// load each key
				gotSite1, err := dl.Load("shippers/1", site1.Name)
				// Stop timer
				t2 := time.Now()
				assert.NilError(t, err)

				// should receive correct sites
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				// should have a result earlier (much earlier) than timeout limit
				assert.Assert(t, t2.Sub(t1) < timeoutLimit)
				// should be two requests
				assert.Equal(t, 1, len(client.recvRequests))
				// should be correct request
				expectedRequest := &freightv1.BatchGetSitesRequest{
					Parent: "shippers/1",
					Names: []string{
						site1.Name,
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
			})

			t.Run("ManyDistinctKeysConcurrently", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				n := 100
				sites := make([]*freightv1.Site, 0, n)
				for i := 0; i < n; i++ {
					sites = append(sites, &freightv1.Site{Name: "shippers/1/sites/" + strconv.Itoa(i)})
				}
				client := &mockSiteService{
					sites: sites,
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					200,
				)

				// protect gotSites
				mu := sync.Mutex{}
				gotSites := make([]*freightv1.Site, 0, n)
				var g errgroup.Group

				// load all keys concurrently
				for i := 0; i < n; i++ {
					i := i
					g.Go(func() error {
						site, err := dl.Load("shippers/1", sites[i].Name)
						if err != nil {
							return err
						}
						mu.Lock()
						defer mu.Unlock()
						gotSites = append(gotSites, site)
						return nil
					})

				}
				assert.NilError(t, g.Wait())

				// should receive correct sites
				assert.DeepEqual(t, sites, gotSites, protocmp.Transform(), cmpopts.SortSlices(siteLessFunc))
				// should only be one request
				assert.Equal(t, 1, len(client.recvRequests))
				// should be the correct names
				expectedNames := make([]string, 0, n)
				for i := 0; i < n; i++ {
					expectedNames = append(expectedNames, sites[i].Name)
				}
				// Needs to be because sites/23 comes before sites/3
				sort.Strings(expectedNames)
				sort.Strings(client.recvRequests[0].Names)
				assert.DeepEqual(t, client.recvRequests[0].Names, expectedNames)
				assert.Equal(t, client.recvRequests[0].Parent, "shippers/1")
			})

			t.Run("DuplicateKeysConcurrently", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// load the same key twice
				var gotSite1 *freightv1.Site
				var gotSite2 *freightv1.Site
				var g errgroup.Group
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site1.Name)
					if err != nil {
						return err
					}
					gotSite1 = site
					return nil
				})
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site1.Name)
					if err != nil {
						return err
					}
					gotSite2 = site
					return nil
				})
				assert.NilError(t, g.Wait())

				// should receive correct site (twice)
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site1, gotSite2, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 1, len(client.recvRequests))
				// should be correct request
				expectedRequest := &freightv1.BatchGetSitesRequest{
					Parent: "shippers/1",
					Names: []string{
						site1.Name,
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
			})

			t.Run("AboveTimeoutLimitConcurrently", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
				sites := []*freightv1.Site{site1, site2}
				client := &mockSiteService{
					sites: sites,
				}
				const timeoutLimit = time.Millisecond * 10
				dl := NewSitesDataloader(
					ctx,
					client,
					timeoutLimit,
					100,
				)

				// load both sites
				var gotSite1 *freightv1.Site
				var gotSite2 *freightv1.Site
				var g errgroup.Group
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site1.Name)
					if err != nil {
						return err
					}
					gotSite1 = site
					return nil
				})
				g.Go(func() error {
					// Sleep to trigger a timeout in the dataloader
					time.Sleep(2 * timeoutLimit)
					site, err := dl.Load("shippers/1", site2.Name)
					if err != nil {
						return err
					}
					gotSite2 = site
					return nil
				})
				assert.NilError(t, g.Wait())

				// should receive correct sites
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site2, gotSite2, protocmp.Transform())
				// should be two requests because of timeout
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
						},
					},
					{
						Parent: "shippers/1",
						Names: []string{
							site2.Name,
						},
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests, protocmp.Transform())
			})
		})
		t.Run("Multi parent", func(t *testing.T) {
			t.Run("Sequential", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site2 := &freightv1.Site{Name: "shippers/2/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1, site2,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)
				gotSite1, err := dl.Load("shippers/1", site1.Name)
				assert.NilError(t, err)
				gotSite2, err := dl.Load("shippers/2", site2.Name)
				assert.NilError(t, err)

				// should receive correct site
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site2, gotSite2, protocmp.Transform())
				// should be two requests
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
						},
					},
					{
						Parent: "shippers/2",
						Names: []string{
							site2.Name,
						},
					},
				}
				assert.DeepEqual(t, expectedRequest, client.recvRequests, protocmp.Transform())
			})
			t.Run("Concurrently", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site2 := &freightv1.Site{Name: "shippers/2/sites/2"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1, site2,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// load the same key twice
				var gotSite1 *freightv1.Site
				var gotSite2 *freightv1.Site
				var g errgroup.Group
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site1.Name)
					if err != nil {
						return err
					}
					gotSite1 = site
					return nil
				})
				g.Go(func() error {
					site, err := dl.Load("shippers/2", site2.Name)
					if err != nil {
						return err
					}
					gotSite2 = site
					return nil
				})
				assert.NilError(t, g.Wait())

				// should receive correct sites
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site2, gotSite2, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
						},
					},
					{
						Parent: "shippers/2",
						Names: []string{
							site2.Name,
						},
					},
				}
				assert.DeepEqual(
					t,
					expectedRequest,
					client.recvRequests,
					protocmp.Transform(),
					cmpopts.SortSlices(batchGetSitesRequestsLessFunc),
				)
			})
			t.Run("Multiple keys per parent", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
				site3 := &freightv1.Site{Name: "shippers/2/sites/1"}
				site4 := &freightv1.Site{Name: "shippers/2/sites/2"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1, site2, site3, site4,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// load the same key twice
				var gotSite1 *freightv1.Site
				var gotSite2 *freightv1.Site
				var gotSite3 *freightv1.Site
				var gotSite4 *freightv1.Site
				var g errgroup.Group
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site1.Name)
					if err != nil {
						return err
					}
					gotSite1 = site
					return nil
				})
				g.Go(func() error {
					site, err := dl.Load("shippers/1", site2.Name)
					if err != nil {
						return err
					}
					gotSite2 = site
					return nil
				})
				g.Go(func() error {
					site, err := dl.Load("shippers/2", site3.Name)
					if err != nil {
						return err
					}
					gotSite3 = site
					return nil
				})
				g.Go(func() error {
					site, err := dl.Load("shippers/2", site4.Name)
					if err != nil {
						return err
					}
					gotSite4 = site
					return nil
				})
				assert.NilError(t, g.Wait())

				// should receive correct sites
				assert.DeepEqual(t, site1, gotSite1, protocmp.Transform())
				assert.DeepEqual(t, site2, gotSite2, protocmp.Transform())
				assert.DeepEqual(t, site3, gotSite3, protocmp.Transform())
				assert.DeepEqual(t, site4, gotSite4, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
							site2.Name,
						},
					},
					{
						Parent: "shippers/2",
						Names: []string{
							site3.Name,
							site4.Name,
						},
					},
				}
				assert.DeepEqual(
					t,
					makeDeterministic(expectedRequest),
					makeDeterministic(client.recvRequests),
					protocmp.Transform(),
					cmpopts.SortSlices(batchGetSitesRequestsLessFunc),
				)
			})
			t.Run("One missing key", func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
				site3 := &freightv1.Site{Name: "shippers/2/sites/1"}
				client := &mockSiteService{
					sites: []*freightv1.Site{
						site1, site3,
					},
				}
				dl := NewSitesDataloader(
					ctx,
					client,
					time.Millisecond*100,
					100,
				)

				// load the same key twice
				var gotSite1Err error
				var gotSite2Err error
				var gotSite3 *freightv1.Site
				var wg sync.WaitGroup
				wg.Add(3)
				go func() {
					_, gotSite1Err = dl.Load("shippers/1", site1.Name)
					wg.Done()
				}()
				go func() {
					_, gotSite2Err = dl.Load("shippers/1", "shippers/1/sites/999")
					wg.Done()
				}()
				go func() {
					gotSite3, _ = dl.Load("shippers/2", site3.Name)
					wg.Done()
				}()
				wg.Wait()

				// requests with parent shipper/1 should fail since second request does not exist
				assert.Equal(t, status.Code(gotSite1Err), codes.NotFound)
				assert.Equal(t, status.Code(gotSite2Err), codes.NotFound)
				assert.DeepEqual(t, site3, gotSite3, protocmp.Transform())
				// should only be one request
				assert.Equal(t, 2, len(client.recvRequests))
				// should be correct request
				expectedRequest := []*freightv1.BatchGetSitesRequest{
					{
						Parent: "shippers/1",
						Names: []string{
							site1.Name,
							"shippers/1/sites/999",
						},
					},
					{
						Parent: "shippers/2",
						Names: []string{
							site3.Name,
						},
					},
				}
				assert.DeepEqual(
					t,
					makeDeterministic(expectedRequest),
					makeDeterministic(client.recvRequests),
					protocmp.Transform(),
					cmpopts.SortSlices(batchGetSitesRequestsLessFunc),
				)
			})
		})
	})

	t.Run("LoadAll", func(t *testing.T) {
		t.Parallel()

		t.Run("DistinctKeys", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
			site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
			sites := []*freightv1.Site{
				site1,
				site2,
			}
			client := &mockSiteService{
				sites: sites,
			}
			dl := NewSitesDataloader(
				ctx,
				client,
				time.Millisecond*100,
				100,
			)

			// load all keys
			gotSites, err := dl.LoadAll("shippers/1", []string{site1.Name, site2.Name})
			assert.NilError(t, err)

			// should receive correct site (twice)
			assert.DeepEqual(t, sites, gotSites, protocmp.Transform())
			// should only be one request
			assert.Equal(t, 1, len(client.recvRequests))
			// should be correct request
			expectedRequest := &freightv1.BatchGetSitesRequest{
				Parent: "shippers/1",
				Names: []string{
					site1.Name,
					site2.Name,
				},
			}
			assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
		})

		t.Run("DistinctAndMissingKeys", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
			site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
			client := &mockSiteService{
				sites: []*freightv1.Site{
					site1,
					site2,
				},
			}
			dl := NewSitesDataloader(
				ctx,
				client,
				time.Millisecond*100,
				100,
			)

			// load all keys
			gotSites, err := dl.LoadAll("shippers/1", []string{site1.Name, "shippers/1/site/999", site2.Name})
			assert.Assert(t, cmp.Nil(gotSites))
			assert.Equal(t, status.Code(err), codes.NotFound)

			// should only be one request
			assert.Equal(t, 1, len(client.recvRequests))
			// should be correct request
			expectedRequest := &freightv1.BatchGetSitesRequest{
				Parent: "shippers/1",
				Names:  []string{site1.Name, "shippers/1/site/999", site2.Name},
			}
			assert.DeepEqual(t, expectedRequest, client.recvRequests[0], protocmp.Transform())
		})

		t.Run("AboveMaxBatchLimit", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
			site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
			sites := []*freightv1.Site{
				site1,
				site2,
			}
			client := &mockSiteService{
				sites: sites,
			}
			const timeoutLimit = time.Millisecond * 100
			const batchLimit = 1
			dl := NewSitesDataloader(
				ctx,
				client,
				timeoutLimit,
				batchLimit,
			)

			// start timer
			t1 := time.Now()
			// load all keys
			gotSites, err := dl.LoadAll("shippers/1", []string{site1.Name, site2.Name})
			// stop timer
			t2 := time.Now()
			assert.NilError(t, err)

			// should receive correct sites
			assert.DeepEqual(t, sites, gotSites, protocmp.Transform())
			// should get result earlier than timeout
			assert.Assert(t, t2.Sub(t1) < timeoutLimit)
			// should be two requests
			assert.Equal(t, 2, len(client.recvRequests))
			// should be correct request
			expectedRequest := []*freightv1.BatchGetSitesRequest{
				{
					Parent: "shippers/1",
					Names: []string{
						site1.Name,
					},
				},
				{
					Parent: "shippers/1",
					Names: []string{
						site2.Name,
					},
				},
			}
			// order is not guaranteed because combination of LoadAll() and end() happens async
			assert.DeepEqual(t, expectedRequest, client.recvRequests, protocmp.Transform(), cmpopts.SortSlices(batchGetSitesRequestsLessFunc))
		})
		t.Run("AboveTimeoutLimitConcurrently", func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			site1 := &freightv1.Site{Name: "shippers/1/sites/1"}
			site2 := &freightv1.Site{Name: "shippers/1/sites/2"}
			site3 := &freightv1.Site{Name: "shippers/1/sites/3"}
			group1 := []*freightv1.Site{site1, site2}
			group2 := []*freightv1.Site{site3}
			client := &mockSiteService{
				sites: []*freightv1.Site{site1, site2, site3},
			}
			const timeoutLimit = time.Millisecond * 10
			dl := NewSitesDataloader(
				ctx,
				client,
				timeoutLimit,
				100,
			)

			// load sites
			var gotSites1 []*freightv1.Site
			var gotSites2 []*freightv1.Site
			var g errgroup.Group
			g.Go(func() error {
				sites, err := dl.LoadAll("shippers/1", []string{site1.Name, site2.Name})
				if err != nil {
					return err
				}
				gotSites1 = sites
				return nil
			})
			g.Go(func() error {
				// Sleep to trigger a timeout in the dataloader
				time.Sleep(timeoutLimit + (time.Millisecond * 5))
				sites, err := dl.LoadAll("shippers/1", []string{site3.Name})
				if err != nil {
					return err
				}
				gotSites2 = sites
				return nil
			})
			assert.NilError(t, g.Wait())

			// should receive correct site (twice)
			assert.DeepEqual(t, group1, gotSites1, protocmp.Transform())
			assert.DeepEqual(t, group2, gotSites2, protocmp.Transform())
			// should only be one request
			assert.Equal(t, 2, len(client.recvRequests))
			// should be correct request
			expectedRequest := []*freightv1.BatchGetSitesRequest{
				{
					Parent: "shippers/1",
					Names:  []string{site1.Name, site2.Name},
				},
				{
					Parent: "shippers/1",
					Names:  []string{site3.Name},
				},
			}
			assert.DeepEqual(t, expectedRequest, client.recvRequests, protocmp.Transform())
		})
	})
}

func siteLessFunc(i, j *freightv1.Site) bool {
	return i.Name < j.Name
}

func batchGetSitesRequestsLessFunc(i, j *freightv1.BatchGetSitesRequest) bool {
	if i.Parent != j.Parent {
		return i.Parent < j.Parent
	}
	if len(i.Names) != len(j.Names) {
		return len(i.Names) < len(j.Names)
	}
	for ii := range i.Names {
		if i.Names[ii] != j.Names[ii] {
			return i.Names[ii] < j.Names[ii]
		}
	}
	return false
}

var _ freightv1.FreightServiceClient = &mockSiteService{}

type mockSiteService struct {
	mu           sync.Mutex // Protect recvRequests below
	recvRequests []*freightv1.BatchGetSitesRequest
	sites        []*freightv1.Site

	// all other methods will panic
	freightv1.FreightServiceClient
}

func (m *mockSiteService) BatchGetSites(
	_ context.Context,
	req *freightv1.BatchGetSitesRequest,
	_ ...grpc.CallOption,
) (*freightv1.BatchGetSitesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recvRequests = append(m.recvRequests, req)
	sites := make([]*freightv1.Site, 0, len(req.GetNames()))
	for _, name := range req.GetNames() {
		for _, site := range m.sites {
			if site.Name == name {
				sites = append(sites, site)
				break
			}
		}
	}
	if len(sites) < len(req.GetNames()) {
		return nil, status.Error(codes.NotFound, "site not found")
	}
	return &freightv1.BatchGetSitesResponse{
		Sites: sites,
	}, nil
}
func makeDeterministic(reqs []*freightv1.BatchGetSitesRequest) []*freightv1.BatchGetSitesRequest {
	sort.SliceStable(reqs, func(i, j int) bool {
		return reqs[i].Parent < reqs[j].Parent
	})
	for _, req := range reqs {
		sort.SliceStable(req.Names, func(i, j int) bool {
			return req.Names[i] < req.Names[j]
		})
	}
	return reqs
}
