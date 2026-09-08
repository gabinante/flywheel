# Workers

Implementation, PR review, feedback, and orchestrator workers are included in the same list as workers you create. All use the same editor and can be renamed, configured, and assigned to tasks. Editing a worker changes new runs that use it, after saving.

Workers → Your workers is the place to define a harness, model, reasoning effort and standing instructions. Save changes before assigning the worker in another page. Task assignments chooses workers for reviews, feedback, implementation, planning and other task types. A workflow step can select its own worker.

Task type describes what a workflow step does and continues to determine its permissions and completion contract. It does not need a custom role to use a custom worker. The worker editor shows the current instructions for its assigned tasks, including their default text, with inline editing. These task prompts are shared by workers doing the same task. Optional Additional instructions apply only to that worker and are prepended to the task prompt. The full prompt library remains available under Task assignments. Harness connections contains shared executable paths, login status and fallback model settings.

## Compatibility and resolution

Previously hidden defaults are exposed as worker definitions, initialized from the existing configuration. Existing custom assignments and project routing are preserved; the seeded workers do not enter the legacy implicit pool of custom workers. Old Models and Prompts links open the corresponding part of Workers. Existing custom roles, rotation and failover policies remain under Advanced routing and legacy roles. Adding a definition preserves existing global task routing rather than implicitly adding the new worker to every task.

Reviews and feedback persist an optional `worker_id`. It takes precedence over legacy `role_id` and service model overrides; blank model/effort use the selected harness defaults. These services accept enabled Codex or Claude workers using shared harness connections. Saving a missing, disabled or unsupported explicit assignment returns an error.

Agent phases persist `config.worker_id`. The dispatcher resolves it from the global library with project definitions overlaid by ID. Missing or disabled explicit selections fail visibly without failover. A selected worker owns the runtime configuration; legacy phase harness/model/effort overrides only apply when the phase uses task assignment. The phase's task-specific instructions and permission contract still apply.

Old API clients and workflows without `worker_id` retain their existing behavior. No database migration is needed because settings and phase configs are JSON.

## Verification

Playwright J09 covers definition editing, reload, review/implementation assignment and invalid assignment rejection. J19 covers selecting a custom worker in a project workflow. The real dispatcher browser journey executes both harness protocols and checks the selected worker's model, effort, instructions, live session and human approval. Go regressions cover direct service selection and project overrides without fallback.
