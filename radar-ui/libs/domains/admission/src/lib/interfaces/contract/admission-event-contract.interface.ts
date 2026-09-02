import { RuleSeverity } from '@cs/domains/rule';

import { AdmissionMonitorPolicyAction } from './admission-monitor-contract.interface';

export enum AdmissionEventCursorDirection {
    LEFT = 'left',
    RIGHT = 'right'
}

export interface AdmissionEventPolicy {
    /** id is "<policy name>/<rule name>". */
    id: string;
    name: string;
    rule: string;
    category: string;
    description: string;
    source: string;
    action: AdmissionMonitorPolicyAction;
}

export interface AdmissionEventThreat {
    policy: AdmissionEventPolicy;
    severity: RuleSeverity;
}

export interface AdmissionEventContainer {
    name: string;
    image_name: string;
    registry: string;
}

export interface AdmissionEventResource {
    api_version: string;
    kind: string;
    namespace: string;
    name: string;
    uid: string;
    node_name: string;
    containers: AdmissionEventContainer[];
}

export interface AdmissionEvent {
    id: string;
    kyverno_version: string;
    registered_at: string; // RFC3339
    resource: AdmissionEventResource;
    threats: AdmissionEventThreat[];
    /** blocked is true when Kyverno itself denied the request. */
    blocked: boolean;
    is_incident: boolean;
    incident_severity: RuleSeverity;
    block_by: string[];
    notify_by: string[];
}

export interface AdmissionDateTimeRange {
    from: string | null; // RFC3339
    to: string | null; // RFC3339
}

export interface AdmissionFilterRequest {
    resource_kind: string[];
    resource_namespace: string[];
    resource_name: string[];
    node_name: string[];
    container_names: string[];
    image_names: string[];
    period: Partial<AdmissionDateTimeRange>;
    blocked?: boolean;
    has_incident?: boolean;
    threats_policies: string[];
    rules: string[];
}

export interface GetAdmissionEventsRequest {
    cursor: string; // RFC3339
    slice_size: number;
}

export interface GetAdmissionEventsByFilterRequest extends GetAdmissionEventsRequest {
    filter: Partial<AdmissionFilterRequest>;
}

export interface GetAdmissionEventsResponse {
    admission_events: AdmissionEvent[];
    left_cursor: string; // RFC3339
    right_cursor: string; // RFC3339
}
