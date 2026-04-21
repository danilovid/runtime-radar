import { Injectable } from '@angular/core';
import { Observable, filter, map, switchMap, take } from 'rxjs';

import { ApiEmptyRequest, ApiService } from '@cs/api';

import {
    CreateIntegrationRequest,
    CreateIntegrationResponse,
    EmptyIntegrationResponse,
    GetIntegrationsResponse,
    IntegrationAI,
    UpdateIntegrationRequest
} from '../interfaces';

export interface TestAIIntegrationRequest {
    integration: CreateIntegrationRequest<IntegrationAI> | IntegrationAI;
}

export interface ExplainRuntimeEventRequest {
    integration_id: string;
    event_id: string;
    event_json: string;
}

export interface ExplainRuntimeEventResponse {
    summary: string;
    risk: string;
    possible_cause: string;
    next_steps: string[];
    raw_text: string;
}

@Injectable({
    providedIn: 'root'
})
export class IntegrationAIRequestService {
    constructor(private readonly apiService: ApiService) {}

    getAIIntegrations(): Observable<IntegrationAI[]> {
        return this.apiService
            .get<ApiEmptyRequest, GetIntegrationsResponse<IntegrationAI>>('integration/ai/list')
            .pipe(map((response) => response.integrations));
    }

    getAIIntegration(id: string): Observable<IntegrationAI> {
        return this.apiService.get<ApiEmptyRequest, IntegrationAI>(`integration/ai/${id}`);
    }

    createAIIntegration(request: CreateIntegrationRequest<IntegrationAI>): Observable<IntegrationAI> {
        return this.apiService
            .post<CreateIntegrationRequest<IntegrationAI>, CreateIntegrationResponse>('integration', request)
            .pipe(
                map((response) => response.id),
                filter((id) => !!id),
                switchMap((id) => this.getAIIntegration(id).pipe(take(1)))
            );
    }

    updateAIIntegration(id: string, request: UpdateIntegrationRequest<IntegrationAI>): Observable<IntegrationAI> {
        return this.apiService
            .patch<UpdateIntegrationRequest<IntegrationAI>, EmptyIntegrationResponse>(`integration/${id}`, request)
            .pipe(
                filter((response) => response && !Object.keys(response).length),
                switchMap(() => this.getAIIntegration(id).pipe(take(1)))
            );
    }

    deleteAIIntegration(id: string): Observable<string> {
        return this.apiService
            .delete<EmptyIntegrationResponse>(`integration/ai/${id}`)
            .pipe(map((response) => (response && !Object.keys(response).length ? id : '')));
    }

    testAIIntegration(request: TestAIIntegrationRequest): Observable<boolean> {
        return this.apiService
            .post<TestAIIntegrationRequest, EmptyIntegrationResponse>('integration/ai/test', request)
            .pipe(map((response) => !!response && !Object.keys(response).length));
    }

    explainRuntimeEvent(request: ExplainRuntimeEventRequest): Observable<ExplainRuntimeEventResponse> {
        return this.apiService.post<ExplainRuntimeEventRequest, ExplainRuntimeEventResponse>(
            'integration/ai/explain-runtime-event',
            request
        );
    }
}
