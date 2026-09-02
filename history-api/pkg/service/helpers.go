package service

import (
	"fmt"
	"strings"

	"github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/history-api/pkg/database/clickhouse"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// likeEscaper neutralizes the SQL LIKE wildcards a user could inject.
	likeEscaper = strings.NewReplacer(
		"%", "", // avoid malicious requests
		"_", `\_`, // escape special symbol _
	)
	// likeGlobs translates the globs of our own filter syntax into SQL LIKE wildcards.
	likeGlobs = strings.NewReplacer(
		"**", "%",
		"*", "%",
		"?", "_",
	)
)

// prepareLikeTemplate converts a filter value into a pattern usable in a SQL LIKE expression.
// Replacers keep no per-call state and are initialized once.
func prepareLikeTemplate(s string) string {
	return likeGlobs.Replace(likeEscaper.Replace(s))
}

// makeRuntimeEventFilter prepares values and patterns from rf to be used in SQL query and constructs gorm expression.
// nolint:goconst
func makeRuntimeEventFilter(rf *api.RuntimeFilter) (clause.Expr, error) {
	var (
		eventTypeEq         = make([]string, 0, len(rf.GetEventType()))
		kprobeFuncLike      = make([]string, 0, len(rf.GetKprobeFunctionName()))
		podNamespaceLike    = make([]string, 0, len(rf.GetProcessPodNamespace()))
		podNameLike         = make([]string, 0, len(rf.GetProcessPodName()))
		nodeNameLike        = make([]string, 0, len(rf.GetNodeName()))
		containerNameLike   = make([]string, 0, len(rf.GetProcessPodContainerName()))
		imageNameLike       = make([]string, 0, len(rf.GetProcessPodContainerImageName()))
		processBinaryLike   = make([]string, 0, len(rf.GetProcessBinary()))
		processArgsLike     = make([]string, 0, len(rf.GetProcessArguments()))
		threatsDetectorsHas = make([]string, 0, len(rf.GetThreatsDetectors()))
		rulesHas            = make([]string, 0, len(rf.GetRules()))
	)

	argsNum := len(rf.GetEventType()) +
		len(rf.GetKprobeFunctionName()) +
		len(rf.GetProcessPodNamespace()) +
		len(rf.GetProcessPodName()) +
		len(rf.GetNodeName()) +
		len(rf.GetProcessPodContainerName()) +
		len(rf.GetProcessPodContainerImageName()) +
		len(rf.GetProcessBinary()) +
		len(rf.GetProcessArguments()) +
		len(rf.GetThreatsDetectors()) +
		len(rf.GetRules())*2 // we're searching in block_by and notify_by at the same time so x2 args are needed

	if rf.GetProcessExecId() != "" {
		argsNum++
	}
	if rf.GetProcessParentExecId() != "" {
		argsNum++
	}

	args := make([]interface{}, 0, argsNum)

	for _, t := range rf.GetEventType() {
		eventTypeEq = append(eventTypeEq, "event_type = ?")
		args = append(args, t)
	}

	for _, fn := range rf.GetKprobeFunctionName() {
		kprobeFuncLike = append(kprobeFuncLike, "kprobe_function_name LIKE ?")
		args = append(args, prepareLikeTemplate(fn))
	}

	for _, pns := range rf.GetProcessPodNamespace() {
		podNamespaceLike = append(podNamespaceLike, "process_pod_namespace LIKE ?")
		args = append(args, prepareLikeTemplate(pns))
	}

	for _, pn := range rf.GetProcessPodName() {
		podNameLike = append(podNameLike, "process_pod_name LIKE ?")
		args = append(args, prepareLikeTemplate(pn))
	}

	for _, nn := range rf.GetNodeName() {
		nodeNameLike = append(nodeNameLike, "node_name LIKE ?")
		args = append(args, prepareLikeTemplate(nn))
	}

	for _, cn := range rf.GetProcessPodContainerName() {
		containerNameLike = append(containerNameLike, "process_pod_container_name LIKE ?")
		args = append(args, prepareLikeTemplate(cn))
	}

	for _, in := range rf.GetProcessPodContainerImageName() {
		imageNameLike = append(imageNameLike, "process_pod_container_image_name LIKE ?")
		args = append(args, prepareLikeTemplate(in))
	}

	for _, b := range rf.GetProcessBinary() {
		processBinaryLike = append(processBinaryLike, "process_binary LIKE ?")
		args = append(args, prepareLikeTemplate(b))
	}

	for _, a := range rf.GetProcessArguments() {
		processArgsLike = append(processArgsLike, "process_arguments LIKE ?")
		args = append(args, prepareLikeTemplate(a))
	}

	for _, td := range rf.GetThreatsDetectors() {
		threatsDetectorsHas = append(threatsDetectorsHas, "has(threats_detectors, ?)")
		args = append(args, td)
	}

	for _, r := range rf.GetRules() {
		rulesHas = append(rulesHas, "has(block_by, ?) OR has(notify_by, ?)")
		args = append(args, r, r)
	}

	sql := makeAndWhereClause(
		eventTypeEq,
		kprobeFuncLike,
		podNamespaceLike,
		podNameLike,
		nodeNameLike,
		containerNameLike,
		imageNameLike,
		processBinaryLike,
		processArgsLike,
		threatsDetectorsHas,
		rulesHas,
	)

	if from := rf.GetPeriod().GetFrom(); from != nil {
		if sql != "" {
			sql += " AND "
		}

		asTime := from.AsTime()

		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		sql += "registered_at > toDateTime64(?, 9, ?)"
		args = append(args, asTime.Format(clickhouse.DateTimeFormat), asTime.Location().String())
	}

	if to := rf.GetPeriod().GetTo(); to != nil {
		if sql != "" {
			sql += " AND "
		}

		asTime := to.AsTime()

		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		sql += "registered_at < toDateTime64(?, 9, ?)"
		args = append(args, asTime.Format(clickhouse.DateTimeFormat), asTime.Location().String())
	}

	if rf.HasThreats != nil {
		if sql != "" {
			sql += " AND "
		}

		if *rf.HasThreats {
			sql += "threats IS NOT NULL"
		} else {
			sql += "threats IS NULL"
		}
	}

	if execID := rf.GetProcessExecId(); execID != "" {
		if sql != "" {
			sql += " AND "
		}

		sql += "process_exec_id = ?"
		args = append(args, execID)
	}

	if parentExecID := rf.GetProcessParentExecId(); parentExecID != "" {
		if sql != "" {
			sql += " AND "
		}

		sql += "process_parent_exec_id = ?"
		args = append(args, parentExecID)
	}

	if rf.HasIncident != nil {
		if sql != "" {
			sql += " AND "
		}

		if *rf.HasIncident {
			sql += "is_incident"
		} else {
			sql += "NOT is_incident"
		}
	}

	return gorm.Expr(sql, args...), nil
}

// makeAndWhereClause returns SQL string with multiple conditions joint via AND.
// Every subClauses' element must represent slice of conditions which will be joint via OR.
// For example, makeAndWhereClause([]string{"a = b", "c = d"}, []string{"e = f"}) will produce the following expression:
// (a = b OR c = d) AND (e = f)
func makeAndWhereClause(subClauses ...[]string) string {
	parts := make([]string, 0, len(subClauses))

	for _, c := range subClauses {
		if len(c) != 0 {
			part := "(" + strings.Join(c, " OR ") + ")"
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, " AND ")
}

func makeOrder(sorts []*api.Sort) string {
	if len(sorts) == 0 {
		return ""
	}

	sb := &strings.Builder{}

	for i, s := range sorts {
		sb.WriteString(s.Field)
		sb.WriteRune(' ')
		sb.WriteString(s.Key)

		if i < len(sorts)-1 {
			sb.WriteRune(',')
		}
	}

	return sb.String()
}

func makeOrderSlice(sorts []*api.Sort) []string {
	if len(sorts) == 0 {
		return nil
	}

	sb := make([]string, 0, countSortsSize(sorts))

	for _, s := range sorts {
		if len(s.Field) > 0 && len(s.Key) > 0 {
			sb = append(sb, fmt.Sprintf("%s %s", s.Field, s.Key))
		}
	}

	return sb
}

func countSortsSize(sorts []*api.Sort) int {
	count := 0
	for _, s := range sorts {
		if len(s.Field) > 0 && len(s.Key) > 0 {
			count++
		}
	}
	return count
}

// makeAdmissionEventFilter prepares values and patterns from af to be used in SQL query and constructs gorm expression.
// Unlike a runtime event, an admission event describes a whole resource, so containers and images are
// arrays and are matched with arrayExists instead of a plain LIKE.
func makeAdmissionEventFilter(af *api.AdmissionFilter) (clause.Expr, error) {
	var (
		resourceKindEq        = make([]string, 0, len(af.GetResourceKind()))
		resourceNamespaceLike = make([]string, 0, len(af.GetResourceNamespace()))
		resourceNameLike      = make([]string, 0, len(af.GetResourceName()))
		nodeNameLike          = make([]string, 0, len(af.GetNodeName()))
		containerNameLike     = make([]string, 0, len(af.GetContainerNames()))
		imageNameLike         = make([]string, 0, len(af.GetImageNames()))
		threatsPoliciesHas    = make([]string, 0, len(af.GetThreatsPolicies()))
		rulesHas              = make([]string, 0, len(af.GetRules()))
	)

	argsNum := len(af.GetResourceKind()) +
		len(af.GetResourceNamespace()) +
		len(af.GetResourceName()) +
		len(af.GetNodeName()) +
		len(af.GetContainerNames()) +
		len(af.GetImageNames()) +
		len(af.GetThreatsPolicies()) +
		len(af.GetRules())*2 // we're searching in block_by and notify_by at the same time so x2 args are needed

	args := make([]interface{}, 0, argsNum)

	for _, k := range af.GetResourceKind() {
		resourceKindEq = append(resourceKindEq, "resource_kind = ?")
		args = append(args, k)
	}

	for _, ns := range af.GetResourceNamespace() {
		resourceNamespaceLike = append(resourceNamespaceLike, "resource_namespace LIKE ?")
		args = append(args, prepareLikeTemplate(ns))
	}

	for _, n := range af.GetResourceName() {
		resourceNameLike = append(resourceNameLike, "resource_name LIKE ?")
		args = append(args, prepareLikeTemplate(n))
	}

	for _, nn := range af.GetNodeName() {
		nodeNameLike = append(nodeNameLike, "node_name LIKE ?")
		args = append(args, prepareLikeTemplate(nn))
	}

	for _, cn := range af.GetContainerNames() {
		containerNameLike = append(containerNameLike, "arrayExists(x -> x LIKE ?, container_names)")
		args = append(args, prepareLikeTemplate(cn))
	}

	for _, in := range af.GetImageNames() {
		imageNameLike = append(imageNameLike, "arrayExists(x -> x LIKE ?, image_names)")
		args = append(args, prepareLikeTemplate(in))
	}

	for _, tp := range af.GetThreatsPolicies() {
		threatsPoliciesHas = append(threatsPoliciesHas, "has(threats_policies, ?)")
		args = append(args, tp)
	}

	for _, r := range af.GetRules() {
		rulesHas = append(rulesHas, "has(block_by, ?) OR has(notify_by, ?)")
		args = append(args, r, r)
	}

	sql := makeAndWhereClause(
		resourceKindEq,
		resourceNamespaceLike,
		resourceNameLike,
		nodeNameLike,
		containerNameLike,
		imageNameLike,
		threatsPoliciesHas,
		rulesHas,
	)

	if from := af.GetPeriod().GetFrom(); from != nil {
		if sql != "" {
			sql += " AND "
		}

		asTime := from.AsTime()

		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		sql += "registered_at > toDateTime64(?, 9, ?)"
		args = append(args, asTime.Format(clickhouse.DateTimeFormat), asTime.Location().String())
	}

	if to := af.GetPeriod().GetTo(); to != nil {
		if sql != "" {
			sql += " AND "
		}

		asTime := to.AsTime()

		// toDateTime64 has to be used because DateTime64 cannot be automatically converted from string. See https://clickhouse.com/docs/en/sql-reference/data-types/datetime64 for details.
		sql += "registered_at < toDateTime64(?, 9, ?)"
		args = append(args, asTime.Format(clickhouse.DateTimeFormat), asTime.Location().String())
	}

	if af.Blocked != nil {
		if sql != "" {
			sql += " AND "
		}

		if *af.Blocked {
			sql += "blocked"
		} else {
			sql += "NOT blocked"
		}
	}

	if af.HasIncident != nil {
		if sql != "" {
			sql += " AND "
		}

		if *af.HasIncident {
			sql += "is_incident"
		} else {
			sql += "NOT is_incident"
		}
	}

	return gorm.Expr(sql, args...), nil
}
