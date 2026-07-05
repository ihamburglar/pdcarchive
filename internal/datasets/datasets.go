package datasets

import "errors"

var ErrUnknownDataset = errors.New("unknown dataset")

type Dataset struct {
	ID        string
	Name      string
	TableName string
}

var All = []Dataset{
	{ID: "kv7h-kjye", Name: "Contributions to Candidates and Political Committees", TableName: "dataset_contributions"},
	{ID: "tijg-9zyp", Name: "Expenditures by Candidates and Political Committees", TableName: "dataset_expenditures"},
	{ID: "7qr9-q2c9", Name: "Campaign Finance Reporting History", TableName: "dataset_reporting_history"},
	{ID: "3h9x-7bvm", Name: "Campaign Finance Summary", TableName: "dataset_summary"},
	{ID: "3r6b-hsaa", Name: "Debt Reported by Candidates and Political Committees", TableName: "dataset_debt"},
	{ID: "d2ig-r3q4", Name: "Loans to Candidates and Political Committees", TableName: "dataset_loans"},
	{ID: "67cp-h962", Name: "Independent Campaign Expenditures and Electioneering Communications", TableName: "dataset_independent_expenditures"},
	{ID: "mppc-zjn9", Name: "Last Minute Contributions to Candidates and Political Committees", TableName: "dataset_last_minute_contributions"},
	{ID: "8bva-rkeb", Name: "Pledges Reporting History", TableName: "dataset_pledges"},
	{ID: "9kcu-2bem", Name: "Candidate Surplus Funds Reports", TableName: "dataset_surplus_funds_reports"},
	{ID: "ti55-mvy5", Name: "Surplus Funds Expenditures", TableName: "dataset_surplus_funds_expenditures"},
	{ID: "qdtg-6yir", Name: "Contributions to out-of-state political committees", TableName: "dataset_out_of_state_contributions"},
	{ID: "mzg4-pm9n", Name: "Expenditures by out-of-state political committees", TableName: "dataset_out_of_state_expenditures"},
	{ID: "9nnw-c693", Name: "Lobbyist Compensation and Expenses by Source", TableName: "dataset_lobbyist_compensation"},
	{ID: "nuwx-ay5h", Name: "Lobbyist Reporting History", TableName: "dataset_lobbyist_reporting_history"},
	{ID: "xhn7-64im", Name: "Lobbyist Employment Registrations", TableName: "dataset_lobbyist_registrations"},
	{ID: "biux-xiwe", Name: "Lobbyist Employers Summary", TableName: "dataset_lobbyist_employers_summary"},
	{ID: "c4ag-3cmj", Name: "Lobbyist Summary", TableName: "dataset_lobbyist_summary"},
	{ID: "e7sd-jbuy", Name: "Lobbyist Agent Employers", TableName: "dataset_lobbyist_agent_employers"},
	{ID: "bp5b-jrti", Name: "Lobbyist Agents", TableName: "dataset_lobbyist_agents"},
	{ID: "mjwb-szba", Name: "Public Agency Lobbying Totals", TableName: "dataset_public_agency_lobbying"},
	{ID: "ef7g-tyg8", Name: "L7 - Employment of Legislators and State Officials", TableName: "dataset_lobbyist_l7_employment"},
	{ID: "3v2j-kqbi", Name: "Pre-2016 Lobbyist Compensation and Expenses by Source", TableName: "dataset_lobbyist_compensation_pre2016"},
	{ID: "ehbc-shxw", Name: "Financial Affairs Disclosures", TableName: "dataset_financial_affairs_disclosures"},
	{ID: "a4ma-dq6s", Name: "PDC Enforcement Cases", TableName: "dataset_enforcement_cases"},
	{ID: "ub89-7wbv", Name: "PDC Enforcement Case Attachments", TableName: "dataset_enforcement_attachments"},
}

func IDs() []string {
	ids := make([]string, len(All))
	for i, d := range All {
		ids[i] = d.ID
	}
	return ids
}

func ByID(id string) (Dataset, bool) {
	for _, d := range All {
		if d.ID == id {
			return d, true
		}
	}
	return Dataset{}, false
}

func TableName(id string) (string, error) {
	if d, ok := ByID(id); ok {
		return d.TableName, nil
	}
	return "", ErrUnknownDataset
}
