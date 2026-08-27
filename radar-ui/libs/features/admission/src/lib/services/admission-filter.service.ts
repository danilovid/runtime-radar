import { Injectable } from '@angular/core';

import { AdmissionEventFilters, AdmissionFilterRequestNode } from '../interfaces/admission-events.interface';

@Injectable({
    providedIn: 'root'
})
export class AdmissionFeatureFilterService {
    /**
     * convertFiltersToRequestNode drops everything the user left empty: history-api rejects a filter
     * where no field is set, and an empty array would otherwise be sent as a meaningful condition.
     */
    static convertFiltersToRequestNode(filters: AdmissionEventFilters): AdmissionFilterRequestNode {
        const node: AdmissionFilterRequestNode = {};

        if (filters.resourceKind.length) {
            node.resource_kind = filters.resourceKind;
        }
        if (filters.resourceNamespace.length) {
            node.resource_namespace = filters.resourceNamespace;
        }
        if (filters.resourceName.length) {
            node.resource_name = filters.resourceName;
        }
        if (filters.nodeName.length) {
            node.node_name = filters.nodeName;
        }
        if (filters.imageNames.length) {
            node.image_names = filters.imageNames;
        }
        if (filters.blocked) {
            node.blocked = true;
        }
        if (filters.hasIncident) {
            node.has_incident = true;
        }

        return node;
    }

    static isFilterExist(filters: AdmissionEventFilters): boolean {
        return !!Object.keys(AdmissionFeatureFilterService.convertFiltersToRequestNode(filters)).length;
    }
}
