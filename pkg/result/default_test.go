package result

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zyguan/tidb-test-util/pkg/env"
)

type mockResultStore struct {
	*http.ServeMux
	results map[string]Result
}

func (rs *mockResultStore) put(r Result) Result {
	if len(r.ID) == 0 {
		r.ID = strconv.Itoa(len(rs.results) + 1)
	}
	rs.results[r.ID] = r
	return r
}

func (rs *mockResultStore) get(id string) *Result {
	r, ok := rs.results[id]
	if !ok {
		return nil
	}
	return &r
}

func newMockStore() *mockResultStore {
	store := mockResultStore{
		ServeMux: http.NewServeMux(),
		results:  make(map[string]Result),
	}
	store.HandleFunc("/results", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var result Result
			err := json.NewDecoder(r.Body).Decode(&result)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(store.put(result))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	store.HandleFunc("/results/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodPatch:
			result := store.get(filepath.Base(r.URL.Path))
			if result == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(result)
				return
			}
			err := json.NewDecoder(r.Body).Decode(result)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(store.put(*result))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return &store
}

func cleanup() {
	os.Unsetenv(env.TestName)
	os.Unsetenv(env.TestResultID)
	os.Unsetenv(env.TestResultEndpoint)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, env.TestLabelPrefix) {
			continue
		}
		kv := strings.SplitN(kv, "=", 2)
		os.Unsetenv(kv[0])
	}
	TestResultEndpoint = ""
}

func TestInitDefault(t *testing.T) {
	cleanup()
	store := newMockStore()
	server := httptest.NewServer(store)
	defer server.Close()

	defaultResult = nil
	TestResultEndpoint = server.URL

	os.Setenv(env.TestResultID, "foo")

	t.Run("NotExists", func(t *testing.T) {
		r, err := InitDefault()
		require.Error(t, err)
		require.Nil(t, r)
	})

	result := New("test", nil)
	result.ID = "foo"
	store.put(*result)

	t.Run("InitOk", func(t *testing.T) {
		r, err := InitDefault()
		require.NoError(t, err)
		require.Equal(t, result, r)
	})
}

func TestResultReport(t *testing.T) {
	cleanup()
	store := newMockStore()
	server := httptest.NewServer(store)
	defer server.Close()

	defaultResult = nil

	t.Run("Uninitialized", func(t *testing.T) {
		require.EqualError(t, Report(Unknown, ""), "default result is nil")
	})

	r, err := InitDefault()
	require.NoError(t, err)
	require.Equal(t, defaultResult, r)
	require.Empty(t, defaultResult.ID)

	t.Run("EmptyEndpoint", func(t *testing.T) {
		err := Report(Unknown, "")
		require.Error(t, err)
		require.True(t, isEmptyEndpointError(err))
	})

	TestResultEndpoint = server.URL

	t.Run("ReportOk", func(t *testing.T) {
		require.NoError(t, Report(Success, "ok"))
		result := store.get(defaultResult.ID)
		require.Equal(t, defaultResult.Output, result.Output)
	})
}
