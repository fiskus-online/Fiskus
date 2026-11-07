package api

import (
	"net/http"
	"strconv"

	"github.com/fiskus-online/fiskus/backend/internal/app"
	"github.com/fiskus-online/fiskus/backend/internal/payroll"
)

// Mzdy: /api/companies/{cid}/employees a /api/companies/{cid}/payroll/{year}/{month}

func (s *Server) listEmployees(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	list, err := s.App.Employees(r.Context(), cid)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) saveEmployee(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	var e payroll.Employee
	if !decode(w, r, &e) {
		return
	}
	e.ID = 0
	if id := r.PathValue("id"); id != "" {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Neplatné ID", nil)
			return
		}
		e.ID = n
	}
	out, err := s.App.SaveEmployee(r.Context(), cid, e)
	if err != nil {
		fail(w, err)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	writeJSON(w, status, out)
}

func (s *Server) deleteEmployee(w http.ResponseWriter, r *http.Request) {
	cid, ok := s.company(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "Neplatné ID", nil)
		return
	}
	if err := s.App.DeleteEmployee(r.Context(), cid, id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// payrollPeriod prečíta firmu, rok a mesiac z adresy.
func (s *Server) payrollPeriod(w http.ResponseWriter, r *http.Request) (int64, int, int, bool) {
	cid, year, ok := s.yearCloseYear(w, r)
	if !ok {
		return 0, 0, 0, false
	}
	month, err := strconv.Atoi(r.PathValue("month"))
	if err != nil || month < 1 || month > 12 {
		writeErr(w, http.StatusBadRequest, "Neplatný mesiac", nil)
		return 0, 0, 0, false
	}
	return cid, year, month, true
}

func (s *Server) payrollResult(w http.ResponseWriter, v app.PayrollView, err error) {
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) getPayroll(w http.ResponseWriter, r *http.Request) {
	if cid, year, month, ok := s.payrollPeriod(w, r); ok {
		v, err := s.App.Payroll(r.Context(), cid, year, month)
		s.payrollResult(w, v, err)
	}
}

// putPayroll: PUT …/payroll/{year}/{month} {inputs: […]}
func (s *Server) putPayroll(w http.ResponseWriter, r *http.Request) {
	cid, year, month, ok := s.payrollPeriod(w, r)
	if !ok {
		return
	}
	var in struct {
		Inputs []payroll.Input `json:"inputs"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.App.SavePayroll(r.Context(), cid, year, month, in.Inputs)
	s.payrollResult(w, v, err)
}

func (s *Server) closePayroll(w http.ResponseWriter, r *http.Request) {
	if cid, year, month, ok := s.payrollPeriod(w, r); ok {
		v, err := s.App.ClosePayroll(r.Context(), cid, year, month)
		s.payrollResult(w, v, err)
	}
}

// reopenPayroll: POST …/payroll/{year}/{month}/reopen {reason}
func (s *Server) reopenPayroll(w http.ResponseWriter, r *http.Request) {
	cid, year, month, ok := s.payrollPeriod(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.App.ReopenPayroll(r.Context(), cid, year, month, in.Reason)
	s.payrollResult(w, v, err)
}
