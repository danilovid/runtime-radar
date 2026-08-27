import { Injectable } from '@angular/core';
import { Observable, filter, map, switchMap, take } from 'rxjs';

import { ApiEmptyRequest, ApiService } from '@cs/api';

import {
    AdmissionEvent,
    AdmissionEventCursorDirection,
    AdmissionMonitor,
    AdmissionMonitorConfig,
    CreateAdmissionMonitorRequest,
    EmptyAdmissionResponse,
    GetAdmissionEventsByFilterRequest,
    GetAdmissionEventsRequest,
    GetAdmissionEventsResponse
} from '../interfaces';

@Injectable({
    providedIn: 'root'
})
export class AdmissionRequestService {
    constructor(private readonly apiService: ApiService) {}

    getAdmissionMonitor(): Observable<AdmissionMonitor> {
        return this.apiService.get<ApiEmptyRequest, AdmissionMonitor>('config/admission-monitor');
    }

    createAdmissionMonitor(config: AdmissionMonitorConfig): Observable<AdmissionMonitor> {
        return this.apiService
            .post<CreateAdmissionMonitorRequest, EmptyAdmissionResponse>('config/admission-monitor', { config })
            .pipe(
                map((response) => response && !Object.keys(response).length),
                filter((isCreated) => isCreated),
                switchMap(() => this.getAdmissionMonitor().pipe(take(1)))
            );
    }

    getEvents(
        direction: AdmissionEventCursorDirection,
        request: GetAdmissionEventsRequest
    ): Observable<GetAdmissionEventsResponse> {
        return this.apiService.get<GetAdmissionEventsRequest, GetAdmissionEventsResponse>(
            `admission-event/slice/${direction}`,
            request
        );
    }

    getEventsByFilter(
        direction: AdmissionEventCursorDirection,
        request: GetAdmissionEventsByFilterRequest
    ): Observable<GetAdmissionEventsResponse> {
        return this.apiService.post<GetAdmissionEventsByFilterRequest, GetAdmissionEventsResponse>(
            `admission-event/by-filter/slice/${direction}`,
            request
        );
    }

    getEvent(id: string): Observable<AdmissionEvent> {
        return this.apiService.get<ApiEmptyRequest, AdmissionEvent>(`admission-event/${id}`);
    }
}
