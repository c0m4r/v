package web

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/c0m4r/v/engine"
)

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}

type vmResponse struct {
	*engine.VM
	State engine.State `json:"state"`
	IP    string       `json:"ip"`
}

func handleListVMs(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vms, err := e.ListVMs()
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}

		result := make([]vmResponse, 0, len(vms))
		for _, vm := range vms {
			state, _ := e.VMState(vm.ID)
			ip := ""
			if state == engine.StateRunning {
				ip = e.VMIPAddress(vm)
			}
			result = append(result, vmResponse{VM: vm, State: state, IP: ip})
		}
		jsonResponse(w, 200, result)
	}
}

func handleCreateVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var opts engine.CreateVMOpts
		if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
			jsonError(w, 400, "invalid JSON: "+err.Error())
			return
		}

		vm, err := e.CreateVM(opts)
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}

		jsonResponse(w, 201, vmResponse{VM: vm, State: engine.StateStopped})
	}
}

func handleSetVMPassword(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, 400, "invalid JSON")
			return
		}
		if err := e.SetRootPassword(r.PathValue("id"), req.Password); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "ok"})
	}
}

func handleGetVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vm, err := e.GetVM(r.PathValue("id"))
		if err != nil {
			jsonError(w, 404, err.Error())
			return
		}

		state, _ := e.VMState(vm.ID)
		ip := ""
		if state == engine.StateRunning {
			ip = e.VMIPAddress(vm)
		}
		jsonResponse(w, 200, vmResponse{VM: vm, State: state, IP: ip})
	}
}

func handleStartVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := e.StartVM(r.PathValue("id")); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "started"})
	}
}

func handleStopVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := e.StopVM(r.PathValue("id")); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "stopping"})
	}
}

func handleForceStopVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := e.ForceStopVM(r.PathValue("id")); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "stopped"})
	}
}

func handleRestartVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := e.RestartVM(r.PathValue("id")); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "restarted"})
	}
}

func handleDeleteVM(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := e.DeleteVM(r.PathValue("id")); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "deleted"})
	}
}

func handleSetBoot(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Dev string `json:"dev"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, 400, "invalid JSON")
			return
		}
		if err := e.SetBootDev(r.PathValue("id"), req.Dev); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"boot_dev": req.Dev})
	}
}

func handleListImages(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		images, err := e.ListImages()
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]any{
			"cached":    images,
			"available": e.KnownImages(),
		})
	}
}

func handlePullImage(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, 400, "invalid JSON")
			return
		}
		if req.Name == "" {
			jsonError(w, 400, "name is required")
			return
		}

		path, err := e.PullImage(req.Name, nil)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}

		jsonResponse(w, 200, map[string]string{"path": path})
	}
}

func handleNetStatus(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, e.GetNetStatus())
	}
}

func handleInfo() http.HandlerFunc {
	isRoot := os.Getuid() == 0
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, 200, map[string]bool{"is_root": isRoot})
	}
}

func handleGetConfig(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := e.LoadConfig()
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, cfg)
	}
}

func handleSetConfig(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cfg engine.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			jsonError(w, 400, "invalid JSON: "+err.Error())
			return
		}
		if err := e.SaveConfig(&cfg); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, cfg)
	}
}
