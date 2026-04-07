---

description: "Task list for Article Publishing feature implementation"
---

# Tasks: Article Publishing

**Input**: Design documents from `/specs/001-article-publish/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Included - feature specification includes comprehensive acceptance scenarios requiring test coverage

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Web application**: Flask app at repository root, services/ in services/, templates/ in templates/
- **Tests**: tests/ directory with existing test structure
- All paths shown are absolute paths from repository root

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project setup and preparation for article publishing feature

- [x] T001 Create backup of current working state before implementing feature
- [x] T002 Verify python-frontmatter dependency is available in requirements.txt
- [x] T003 Review existing PostService structure and identify extension points
- [x] T004 [P] Review existing API structure in app.py to understand endpoint patterns
- [x] T005 [P] Review existing template structure in templates/ to understand UI patterns

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core service layer extensions that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T006 Add publish_article method to PostService in services/post_service.py
- [x] T007 Add bulk_publish_articles method to PostService in services/post_service.py
- [x] T008 Add get_publish_status method to PostService in services/post_service.py
- [x] T009 Add file locking helper methods to PostService for concurrency handling
- [x] T010 Add frontmatter validation helpers to PostService in services/post_service.py
- [x] T011 Add file path validation helpers to PostService in services/post_service.py

**Checkpoint**: PostService foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Publish Article from Editor (Priority: P1) 🎯 MVP

**Goal**: Add publish button to article editor that changes draft status from true to false

**Independent Test**: Can publish an article in editor and verify draft status changes correctly in the file

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T012 [P] [US1] Create test_publish_api.py in tests/ with test_publish_article_success
- [x] T013 [P] [US1] Create test_publish_api.py with test_publish_already_published
- [x] T014 [P] [US1] Create test_publish_api.py with test_get_article_status
- [x] T015 [P] [US1] Create test_post_service.py with test_publish_article_service
- [x] T016 [P] [US1] Create test_publish_integration.py with test_full_publish_workflow

### Implementation for User Story 1

- [x] T017 [US1] Add POST /api/article/publish endpoint in app.py (depends on T006, T009, T010, T011)
- [x] T018 [US1] Add GET /api/article/status endpoint in app.py (depends on T008)
- [x] T019 [US1] Add publish button to templates/editor.html near existing save buttons
- [x] T020 [US1] Add JavaScript confirmation dialog for publish action in templates/editor.html
- [x] T021 [US1] Add publish status loading logic in templates/editor.html to disable button for already published articles
- [x] T022 [US1] Add HTMX attributes to publish button for API integration in templates/editor.html
- [x] T023 [US1] Add publish status display element in templates/editor.html
- [x] T024 [US1] Add error handling and user feedback for publish operations in templates/editor.html

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently

---

## Phase 4: User Story 2 - Visual Publish Status Indicators (Priority: P2)

**Goal**: Add visual badges in article list to show draft vs published status

**Independent Test**: Can load article list and verify draft and published articles have distinct visual indicators

### Tests for User Story 2

- [ ] T025 [P] [US2] Create test_status_api.py with test_get_bulk_article_status
- [ ] T026 [P] [US2] Create test_status_ui.py with test_status_indicators_rendering

### Implementation for User Story 2

- [ ] T027 [US2] Add GET /api/article/status/bulk endpoint in app.py for multiple article status lookup
- [ ] T028 [US2] Add status badge placeholder elements to article listing in templates/posts.html
- [ ] T029 [US2] Add JavaScript to load and display status indicators for each article in templates/posts.html
- [ ] T030 [US2] Add CSS styling for draft (yellow) and published (green) status badges in templates/posts.html
- [ ] T031 [US2] Add error handling for status loading failures in templates/posts.html

**Checkpoint**: User Story 2 complete - users can see article status at a glance

---

## Phase 5: User Story 3 - Bulk Publishing Operations (Priority: P3)

**Goal**: Add bulk publish functionality to publish multiple articles from list view

**Independent Test**: Can select multiple draft articles and publish them in one operation

### Tests for User Story 3

- [ ] T032 [P] [US3] Create test_bulk_publish_api.py with test_bulk_publish_success
- [ ] T033 [P] [US3] Create test_bulk_publish_api.py with test_bulk_publish_partial_failures
- [ ] T034 [P] [US3] Create test_bulk_publish_ui.py with test_bulk_publish_workflow

### Implementation for User Story 3

- [ ] T035 [US3] Add POST /api/article/publish/bulk endpoint in app.py (depends on T007)
- [ ] T036 [US3] Add checkbox selection UI to article list in templates/posts.html
- [ ] T037 [US3] Add "Select All" checkbox functionality in templates/posts.html
- [ ] T038 [US3] Add bulk publish button with selection counter in templates/posts.html
- [ ] T039 [US3] Add bulk publish confirmation dialog in templates/posts.html
- [ ] T040 [US3] Add bulk publish results display with success/failure summary in templates/posts.html
- [ ] T041 [US3] Add page reload after successful bulk publish to refresh status indicators in templates/posts.html

**Checkpoint**: User Story 3 complete - users can efficiently publish multiple articles

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final touches, performance optimization, and comprehensive testing

### Performance & Caching

- [ ] T042 Add cache invalidation for published articles in PostService publish methods
- [ ] T043 Optimize bulk operations for better performance with large article sets
- [ ] T044 Add request timeout handling for long-running bulk operations

### Error Handling & Edge Cases

- [ ] T045 Add handling for articles without frontmatter in publish operations
- [ ] T046 Add handling for malformed YAML frontmatter in publish operations
- [ ] T047 Add handling for concurrent publish attempts on same article
- [ ] T048 Add handling for file permission errors during publish operations
- [ ] T049 Add handling for deleted files during publish operations

### Security & Validation

- [ ] T050 Add input validation for file paths to prevent directory traversal attacks
- [ ] T051 Add permission checks to ensure users can only publish allowed articles
- [ ] T052 Add logging for all publish operations for audit trail

### Testing & Quality Assurance

- [ ] T053 Run full test suite and ensure all tests pass
- [ ] T054 Test with real Hugo content files to ensure compatibility
- [ ] T055 Test concurrent publish scenarios to verify file locking works correctly
- [ ] T056 Test error scenarios and verify user-friendly error messages
- [ ] T057 Test bulk operations with large numbers of articles

### Documentation

- [ ] T058 Update API documentation with new endpoints
- [ ] T059 Add usage examples to project documentation
- [ ] T060 Document troubleshooting steps for common publish issues

**Checkpoint**: Feature complete and production-ready

---

## Dependencies & Execution Strategy

### Story Dependencies
```
Phase 2 (Foundational) → Phase 3 (US1) → {Phase 4 (US2), Phase 5 (US3)} → Phase 6 (Polish)
```

**Key Dependencies**:
- **Phase 2** blocks ALL user stories (PostService extensions required)
- **US1 (P1)** blocks US2 and US3 (publish functionality is prerequisite)
- **US2** and **US3** can be developed in parallel after US1 is complete
- **Phase 6** depends on all user stories being complete

### Parallel Execution Opportunities

**Phase 1 (Setup)**:
- T004, T005 can run in parallel (review existing code)

**Phase 2 (Foundational)**:
- T006, T007, T008 can run in parallel (different PostService methods)
- T009, T010, T011 can run in parallel (different helper methods)

**Phase 3 (US1)**:
- T012, T013, T014, T015, T016 can run in parallel (test creation)
- T019, T020, T021, T022, T023, T024 can run in parallel (UI components)

**Phase 4 (US2)**:
- T025, T026 can run in parallel (test creation)
- T028, T029, T030, T031 can run in parallel (UI components)

**Phase 5 (US3)**:
- T032, T033, T034 can run in parallel (test creation)
- T036, T037, T038, T039, T040, T041 can run in parallel (UI components)

**Phase 6 (Polish)**:
- Most tasks can run in parallel as they address different aspects

### Implementation Strategy

**MVP First**: Deliver User Story 1 (Phase 3) as MVP - this provides core publish functionality

**Incremental Delivery**: Each user story phase delivers complete, independently testable functionality

**Risk Mitigation**:
- Start with PostService extensions (Phase 2) to validate technical approach
- Focus on file locking and error handling early to avoid concurrency issues
- Test with real Hugo content to ensure compatibility

### Success Criteria

**Phase Completion**:
- Each phase completes with working, testable functionality
- All acceptance scenarios from specification are covered
- Performance requirements met (<3 second publish time)
- Error handling covers all identified edge cases

**Feature Complete**:
- All user stories implemented and tested
- Production-ready with comprehensive error handling
- Documentation complete
- Performance optimized for target scale (hundreds of articles)

---

**Total Tasks**: 60
**Tasks per User Story**:
- US1 (P1): 15 tasks (including tests)
- US2 (P2): 7 tasks (including tests)
- US3 (P3): 9 tasks (including tests)
- Shared/Polish: 29 tasks

**Estimated Timeline**: 4-5 days
- Phase 1-2: 1 day (setup + foundation)
- Phase 3: 1-2 days (core publish functionality)
- Phase 4: 1 day (status indicators)
- Phase 5: 1 day (bulk publishing)
- Phase 6: 0.5 day (polish and testing)
