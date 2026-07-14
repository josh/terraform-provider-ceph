package main

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	_ resource.Resource                   = &PoolResource{}
	_ resource.ResourceWithImportState    = &PoolResource{}
	_ resource.ResourceWithValidateConfig = &PoolResource{}
)

func newPoolResource() resource.Resource {
	return &PoolResource{}
}

type PoolResource struct {
	client *restapi.Client
}

type PoolResourceModel struct {
	Name                     types.String   `tfsdk:"name"`
	PoolType                 types.String   `tfsdk:"pool_type"`
	PgNum                    types.Int64    `tfsdk:"pg_num"`
	PgpNum                   types.Int64    `tfsdk:"pgp_num"`
	CrushRule                types.String   `tfsdk:"crush_rule"`
	ErasureCodeProfile       types.String   `tfsdk:"erasure_code_profile"`
	MinSize                  types.Int64    `tfsdk:"min_size"`
	Size                     types.Int64    `tfsdk:"size"`
	PgAutoscaleMode          types.String   `tfsdk:"pg_autoscale_mode"`
	QuotaMaxObjects          types.Int64    `tfsdk:"quota_max_objects"`
	QuotaMaxBytes            types.Int64    `tfsdk:"quota_max_bytes"`
	CompressionMode          types.String   `tfsdk:"compression_mode"`
	CompressionAlgorithm     types.String   `tfsdk:"compression_algorithm"`
	CompressionRequiredRatio types.Float64  `tfsdk:"compression_required_ratio"`
	CompressionMinBlobSize   types.Int64    `tfsdk:"compression_min_blob_size"`
	CompressionMaxBlobSize   types.Int64    `tfsdk:"compression_max_blob_size"`
	ECOverwrites             types.Bool     `tfsdk:"ec_overwrites"`
	Configuration            types.Map      `tfsdk:"configuration"`
	PoolID                   types.Int64    `tfsdk:"pool_id"`
	ApplicationMetadata      types.Set      `tfsdk:"application_metadata"`
	Timeouts                 timeouts.Value `tfsdk:"timeouts"`
}

func (r *PoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *PoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceSchema.Schema{
		MarkdownDescription: "This resource manages a Ceph pool.",
		Attributes: map[string]resourceSchema.Attribute{
			"name": resourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool.",
				Required:            true,
			},
			"pool_type": resourceSchema.StringAttribute{
				MarkdownDescription: "The type of pool. Must be either 'replicated' or 'erasure'.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("replicated", "erasure"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pg_num": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of placement groups for the pool. Conflicts with `pg_autoscale_mode = \"on\"`; exactly one of the two must decide the placement group count.",
				Optional:            true,
				Computed:            true,
			},
			"pgp_num": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of placement groups for placement.",
				Optional:            true,
				Computed:            true,
			},
			"crush_rule": resourceSchema.StringAttribute{
				MarkdownDescription: "The crush rule for the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"erasure_code_profile": resourceSchema.StringAttribute{
				MarkdownDescription: "The erasure code profile of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"min_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The minimum number of replicas for the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of replicas for the pool.",
				Optional:            true,
				Computed:            true,
			},
			"pg_autoscale_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The placement group autoscale mode. Must be one of: 'off', 'warn', or 'on'. When 'on', pg_num must not be set.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("off", "warn", "on"),
				},
			},
			"quota_max_objects": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of objects allowed in the pool (hard limit).",
				Optional:            true,
				Computed:            true,
			},
			"quota_max_bytes": resourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum bytes allowed in the pool (hard limit).",
				Optional:            true,
				Computed:            true,
			},
			"compression_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The compression mode of the pool. Must be one of: 'none', 'passive', 'aggressive', or 'force'.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("none", "passive", "aggressive", "force"),
				},
			},
			"compression_algorithm": resourceSchema.StringAttribute{
				MarkdownDescription: "The compression algorithm of the pool.",
				Optional:            true,
				Computed:            true,
			},
			"compression_required_ratio": resourceSchema.Float64Attribute{
				MarkdownDescription: "The compression required ratio of the pool.",
				Optional:            true,
				Computed:            true,
			},
			"compression_min_blob_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The compression minimum blob size of the pool.",
				Optional:            true,
				Computed:            true,
			},
			"compression_max_blob_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The compression maximum blob size of the pool.",
				Optional:            true,
				Computed:            true,
			},
			"ec_overwrites": resourceSchema.BoolAttribute{
				MarkdownDescription: "Whether overwrites are allowed on the erasure coded pool, required to run RBD or CephFS on it. Defaults to false. Ceph cannot unset the flag, so disabling requires destroying and recreating the pool.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplaceIf(
						func(ctx context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = req.StateValue.ValueBool() && !req.PlanValue.ValueBool()
						},
						"Disabling ec_overwrites requires replacing the pool.",
						"Disabling ec_overwrites requires replacing the pool.",
					),
				},
			},
			"configuration": resourceSchema.MapAttribute{
				MarkdownDescription: "Pool-level RBD configuration overrides, e.g. QoS options like `rbd_qos_bps_limit`. Keys must be valid librbd option names; unknown keys are silently ignored by Ceph and would never converge.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the pool.",
				Computed:            true,
			},
			"application_metadata": resourceSchema.SetAttribute{
				MarkdownDescription: "List of application types for the pool (e.g., [\"rbd\", \"rgw\"]).",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *PoolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *PoolResource) updateModelFromAPI(ctx context.Context, data *PoolResourceModel, pool *restapi.Pool) diag.Diagnostics {
	var diags diag.Diagnostics

	data.Name = types.StringValue(pool.PoolName)
	data.PoolType = types.StringValue(pool.Type)
	data.PoolID = types.Int64Value(int64(pool.PoolID))
	data.ECOverwrites = types.BoolValue(slices.Contains(strings.Split(pool.FlagsNames, ","), "ec_overwrites"))

	if pool.Size > 0 {
		data.Size = types.Int64Value(int64(pool.Size))
	} else {
		data.Size = types.Int64Null()
	}

	if pool.MinSize > 0 {
		data.MinSize = types.Int64Value(int64(pool.MinSize))
	} else {
		data.MinSize = types.Int64Null()
	}

	// Ceph applies a pg_num/pgp_num change to its target immediately, then
	// splits or merges the physical count toward it in the background. Track
	// the target so state reflects the requested value as soon as it is
	// accepted, without waiting for (or drifting during) physical convergence.
	if pool.PGAutoscaleMode == "on" {
		// The autoscaler owns both pg_num and pgp_num, so keep the configured
		// values instead of tracking Ceph's to avoid spurious drift, but never
		// leave an unknown planned value in state.
		if data.PgNum.IsUnknown() {
			data.PgNum = types.Int64Null()
		}
		if data.PgpNum.IsUnknown() {
			data.PgpNum = types.Int64Null()
		}
	} else {
		if pgNum := pgNumForState(pool.PGNumTarget, pool.PGNum); pgNum > 0 {
			data.PgNum = types.Int64Value(int64(pgNum))
		} else {
			data.PgNum = types.Int64Null()
		}
		if pgpNum := pgNumForState(pool.PGPlacementNumTarget, pool.PGPlacementNum); pgpNum > 0 {
			data.PgpNum = types.Int64Value(int64(pgpNum))
		} else {
			data.PgpNum = types.Int64Null()
		}
	}

	if pool.CrushRule != "" {
		data.CrushRule = types.StringValue(pool.CrushRule)
	} else {
		data.CrushRule = types.StringNull()
	}

	if pool.ErasureCodeProfile != "" {
		data.ErasureCodeProfile = types.StringValue(pool.ErasureCodeProfile)
	} else {
		data.ErasureCodeProfile = types.StringNull()
	}

	if pool.PGAutoscaleMode != "" {
		data.PgAutoscaleMode = types.StringValue(pool.PGAutoscaleMode)
	} else {
		data.PgAutoscaleMode = types.StringNull()
	}

	if pool.QuotaMaxObjects >= 0 {
		data.QuotaMaxObjects = types.Int64Value(int64(pool.QuotaMaxObjects))
	} else {
		data.QuotaMaxObjects = types.Int64Null()
	}

	if pool.QuotaMaxBytes >= 0 {
		data.QuotaMaxBytes = types.Int64Value(int64(pool.QuotaMaxBytes))
	} else {
		data.QuotaMaxBytes = types.Int64Null()
	}

	if pool.Options.CompressionMode != "" {
		data.CompressionMode = types.StringValue(pool.Options.CompressionMode)
	} else {
		data.CompressionMode = types.StringNull()
	}

	if pool.Options.CompressionAlgorithm != "" {
		data.CompressionAlgorithm = types.StringValue(pool.Options.CompressionAlgorithm)
	} else {
		data.CompressionAlgorithm = types.StringNull()
	}

	if pool.Options.CompressionRequiredRatio > 0 {
		data.CompressionRequiredRatio = types.Float64Value(pool.Options.CompressionRequiredRatio)
	} else {
		data.CompressionRequiredRatio = types.Float64Null()
	}

	if pool.Options.CompressionMinBlobSize > 0 {
		data.CompressionMinBlobSize = types.Int64Value(int64(pool.Options.CompressionMinBlobSize))
	} else {
		data.CompressionMinBlobSize = types.Int64Null()
	}

	if pool.Options.CompressionMaxBlobSize > 0 {
		data.CompressionMaxBlobSize = types.Int64Value(int64(pool.Options.CompressionMaxBlobSize))
	} else {
		data.CompressionMaxBlobSize = types.Int64Null()
	}

	if len(pool.ApplicationMetadata) > 0 {
		apps, d := types.SetValueFrom(ctx, types.StringType, pool.ApplicationMetadata)
		diags.Append(d...)
		data.ApplicationMetadata = apps
	} else {
		data.ApplicationMetadata = types.SetNull(types.StringType)
	}

	poolConfig := map[string]string{}
	for _, option := range pool.Configuration {
		// Source 1 marks pool-level overrides.
		if option.Source == 1 {
			poolConfig[option.Name] = option.Value
		}
	}
	if !data.Configuration.IsNull() || len(poolConfig) > 0 {
		configuration, d := types.MapValueFrom(ctx, types.StringType, poolConfig)
		diags.Append(d...)
		data.Configuration = configuration
	}

	return diags
}

func (r *PoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := data.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	poolType := data.PoolType.ValueString()
	createReq := restapi.PoolCreateRequest{
		Pool:     data.Name.ValueString(),
		PoolType: &poolType,
	}

	if data.ECOverwrites.ValueBool() {
		createReq.Flags = []string{"ec_overwrites"}
	}

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		v := int(data.PgNum.ValueInt64())
		createReq.PgNum = &v
	} else if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() && data.PgAutoscaleMode.ValueString() == "on" {
		defaultPgNum := 1
		createReq.PgNum = &defaultPgNum
	}

	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
		v := int(data.PgpNum.ValueInt64())
		createReq.PgpNum = &v
	}

	if !data.CrushRule.IsNull() && !data.CrushRule.IsUnknown() {
		v := data.CrushRule.ValueString()
		createReq.RuleName = &v
	}

	if !data.ErasureCodeProfile.IsNull() && !data.ErasureCodeProfile.IsUnknown() {
		v := data.ErasureCodeProfile.ValueString()
		createReq.ErasureCodeProfile = &v
	}

	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		v := int(data.MinSize.ValueInt64())
		createReq.MinSize = &v
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		v := int(data.Size.ValueInt64())
		createReq.Size = &v
	}

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		v := data.PgAutoscaleMode.ValueString()
		createReq.PgAutoscaleMode = &v
	}

	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		v := int(data.QuotaMaxObjects.ValueInt64())
		createReq.QuotaMaxObjects = &v
	}

	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		v := int(data.QuotaMaxBytes.ValueInt64())
		createReq.QuotaMaxBytes = &v
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		v := data.CompressionMode.ValueString()
		createReq.CompressionMode = &v
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		v := data.CompressionAlgorithm.ValueString()
		createReq.CompressionAlgorithm = &v
	}

	if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
		v := data.CompressionRequiredRatio.ValueFloat64()
		createReq.CompressionRequiredRatio = &v
	}

	if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
		v := int(data.CompressionMinBlobSize.ValueInt64())
		createReq.CompressionMinBlobSize = &v
	}

	if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
		v := int(data.CompressionMaxBlobSize.ValueInt64())
		createReq.CompressionMaxBlobSize = &v
	}

	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		var apps []string
		data.ApplicationMetadata.ElementsAs(ctx, &apps, false)
		createReq.ApplicationMetadata = apps
	}

	createReq.Configuration = mapWithRemovals(ctx, data.Configuration, types.MapNull(types.StringType), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating pool", map[string]interface{}{
		"name":      data.Name.ValueString(),
		"pool_type": data.PoolType.ValueString(),
	})

	taskInfo, err := r.client.CreatePool(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create pool '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	if taskInfo != nil {
		tflog.Debug(ctx, "Pool creation is async, waiting for task", map[string]interface{}{
			"task_name": taskInfo.Name,
		})
		if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
			resp.Diagnostics.AddError(
				"Task Wait Failed",
				fmt.Sprintf("Failed waiting for pool creation task: %s", err),
			)
			return
		}
	}

	pool, err := r.client.GetPool(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool '%s' after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	// The pool exists from here on, so record it before reasserting pg_num
	// to keep a failure there from orphaning it from state.
	partial := data
	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &partial, pool)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The dashboard turns pg_autoscale_mode off only after the pool exists,
	// so the autoscaler can move pg_num_target during that window. If the
	// requested value did not survive, set it again now that autoscaling is
	// settled. Ceph accepts the new target synchronously and converges the
	// physical pg_num in the background, so there is nothing to wait on
	// beyond the target being visible.
	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() && pool.PGAutoscaleMode != "on" {
		requested := int(data.PgNum.ValueInt64())
		target := pgNumForState(pool.PGNumTarget, pool.PGNum)
		if target != requested {
			tflog.Debug(ctx, "Reasserting pg_num after creation", map[string]interface{}{
				"requested": requested,
				"target":    target,
			})

			// Send pgp_num alongside pg_num when the user pinned it: the
			// dashboard rewrites pgp_num to match pg_num whenever pg_num is
			// set, so omitting it would clobber a distinctly-configured value.
			reassert := restapi.PoolUpdateRequest{PgNum: &requested}
			if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
				pgp := int(data.PgpNum.ValueInt64())
				reassert.PgpNum = &pgp
			}

			if _, err := r.client.UpdatePool(ctx, data.Name.ValueString(), reassert); err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to reassert pg_num for pool '%s' after creation: %s", data.Name.ValueString(), err),
				)
				return
			}

			pool, err = r.waitForPgNumTarget(ctx, data.Name.ValueString(), requested)
			if err != nil {
				resp.Diagnostics.AddError(
					"API Request Error",
					fmt.Sprintf("Unable to confirm pg_num for pool '%s' after reasserting: %s", data.Name.ValueString(), err),
				)
				return
			}
		}
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, pool)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// pgNumForState prefers Ceph's *_target (the value it accepted and converges
// the physical count toward in the background) over the current physical
// count, so state reflects the requested value without drifting during a
// split or merge. It falls back to the physical count on the rare osdmap that
// omits the target.
func pgNumForState(target, physical int) int {
	if target > 0 {
		return target
	}
	return physical
}

// waitForPgNumTarget polls until Ceph reports the requested pg_num as the
// pool's target. The dashboard applies the change inside an async task, so the
// target may not be visible on the first read; it settles within an osdmap
// epoch, long before the physical split or merge finishes.
func (r *PoolResource) waitForPgNumTarget(ctx context.Context, poolName string, requested int) (*restapi.Pool, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		pool, err := r.client.GetPool(ctx, poolName)
		switch {
		case err != nil:
			// The pool already exists, so a transient read failure should not
			// sink an otherwise-successful create; keep polling until the
			// deadline and report the last error if we never recover.
			lastErr = err
		case pool.PGNumTarget == requested || (pool.PGNumTarget == 0 && pool.PGNum == requested):
			return pool, nil
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("pg_num target did not reach %d (last read error: %v): %w", requested, lastErr, ctx.Err())
			}
			return nil, fmt.Errorf("pg_num target did not reach %d: %w", requested, ctx.Err())
		case <-ticker.C:
		}
	}
}

// waitForNoPoolEditTask polls until no pool/edit task for the pool is
// executing. The dashboard identifies tasks by (name, metadata) and returns
// the already-running task instead of applying a new edit, so issuing one
// while a previous edit is still converging would be silently ignored.
func (r *PoolResource) waitForNoPoolEditTask(ctx context.Context, poolName string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		tasks, err := r.client.GetTasks(ctx, "pool/edit")
		if err != nil {
			lastErr = err
		} else {
			busy := false
			for _, task := range tasks.ExecutingTasks {
				if task.Metadata["pool_name"] == poolName {
					busy = true
					break
				}
			}
			if !busy {
				return nil
			}
			tflog.Debug(ctx, "Waiting for running pool edit task to finish", map[string]interface{}{
				"pool_name": poolName,
			})
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("earlier pool edit task still running (last poll error: %v): %w", lastErr, ctx.Err())
			}
			return fmt.Errorf("earlier pool edit task still running: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (r *PoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading pool", map[string]interface{}{
		"name": data.Name.ValueString(),
	})

	poolName := data.Name.ValueString()

	pool, err := r.client.GetPool(ctx, poolName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			tflog.Debug(ctx, "Pool not found, removing from state", map[string]interface{}{
				"pool_name": poolName,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool '%s': %s", poolName, err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, pool)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PoolResourceModel
	var state PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := data.Timeouts.Update(ctx, 30*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	originalPoolName := state.Name.ValueString()
	newPoolName := data.Name.ValueString()

	updateReq := restapi.PoolUpdateRequest{}

	if data.ECOverwrites.ValueBool() && !state.ECOverwrites.ValueBool() {
		updateReq.Flags = []string{"ec_overwrites"}
	}

	// Only send pg counts that actually changed, so updates to unrelated
	// attributes never reset a value the autoscaler has since adjusted.
	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() && !data.PgNum.Equal(state.PgNum) {
		v := int(data.PgNum.ValueInt64())
		updateReq.PgNum = &v
	}

	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() && !data.PgpNum.Equal(state.PgpNum) {
		v := int(data.PgpNum.ValueInt64())
		updateReq.PgpNum = &v
	}

	// Only send size and crush_rule when they actually changed; Ceph rejects
	// setting either on erasure-coded pools, where both are computed.
	if !data.Size.IsNull() && !data.Size.IsUnknown() && !data.Size.Equal(state.Size) {
		v := int(data.Size.ValueInt64())
		updateReq.Size = &v
	}

	if !data.CrushRule.IsNull() && !data.CrushRule.IsUnknown() && !data.CrushRule.Equal(state.CrushRule) {
		v := data.CrushRule.ValueString()
		updateReq.CrushRule = &v
	}

	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		v := int(data.MinSize.ValueInt64())
		updateReq.MinSize = &v
	}

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		v := data.PgAutoscaleMode.ValueString()
		updateReq.PgAutoscaleMode = &v
	}

	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		v := int(data.QuotaMaxObjects.ValueInt64())
		updateReq.QuotaMaxObjects = &v
	}

	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		v := int(data.QuotaMaxBytes.ValueInt64())
		updateReq.QuotaMaxBytes = &v
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		v := data.CompressionMode.ValueString()
		updateReq.CompressionMode = &v
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		v := data.CompressionAlgorithm.ValueString()
		updateReq.CompressionAlgorithm = &v
	}

	if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
		v := data.CompressionRequiredRatio.ValueFloat64()
		updateReq.CompressionRequiredRatio = &v
	}

	if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
		v := int(data.CompressionMinBlobSize.ValueInt64())
		updateReq.CompressionMinBlobSize = &v
	}

	if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
		v := int(data.CompressionMaxBlobSize.ValueInt64())
		updateReq.CompressionMaxBlobSize = &v
	}

	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		var apps []string
		data.ApplicationMetadata.ElementsAs(ctx, &apps, false)
		updateReq.ApplicationMetadata = apps
	}

	if !data.Configuration.Equal(state.Configuration) {
		updateReq.Configuration = mapWithRemovals(ctx, data.Configuration, state.Configuration, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if originalPoolName != newPoolName {
		updateReq.Pool = &newPoolName
	} else {
		updateReq.Pool = nil
	}

	tflog.Debug(ctx, "Updating pool", map[string]interface{}{
		"original_name": originalPoolName,
		"new_name":      newPoolName,
	})

	// The dashboard deduplicates tasks by (name, metadata) and silently drops
	// an edit that arrives while another edit on the same pool is executing,
	// e.g. the create-time pg_num reassert still converging its PG merge. Wait
	// for the pool's edit slot to free up so this update is actually applied.
	if err := r.waitForNoPoolEditTask(ctx, originalPoolName); err != nil {
		resp.Diagnostics.AddError(
			"Task Wait Failed",
			fmt.Sprintf("Unable to update pool '%s' while an earlier edit is still running: %s", originalPoolName, err),
		)
		return
	}

	taskInfo, err := r.client.UpdatePool(ctx, originalPoolName, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update pool '%s': %s", originalPoolName, err),
		)
		return
	}

	if taskInfo != nil {
		tflog.Debug(ctx, "Pool update is async, waiting for task", map[string]interface{}{
			"task_name": taskInfo.Name,
		})
		if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
			resp.Diagnostics.AddError(
				"Task Wait Failed",
				fmt.Sprintf("Failed waiting for pool update task: %s", err),
			)
			return
		}
	}

	pool, err := r.client.GetPool(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool '%s' after update: %s", data.Name.ValueString(), err),
		)
		return
	}

	resp.Diagnostics.Append(r.updateModelFromAPI(ctx, &data, pool)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := data.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	poolName := data.Name.ValueString()

	tflog.Debug(ctx, "Deleting pool", map[string]interface{}{
		"name": poolName,
	})

	taskInfo, err := r.client.DeletePool(ctx, poolName)
	if err != nil {
		if errors.Is(err, restapi.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete pool '%s': %s", data.Name.ValueString(), err),
		)
		return
	}

	if taskInfo != nil {
		tflog.Debug(ctx, "Pool deletion is async, waiting for task", map[string]interface{}{
			"task_name": taskInfo.Name,
		})
		if err := r.client.WaitForTask(ctx, taskInfo); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				resp.Diagnostics.AddWarning(
					"Pool Deletion Task Timeout",
					fmt.Sprintf("Pool deletion task did not complete within timeout. Deletion may eventually complete: %s", err),
				)
			} else {
				resp.Diagnostics.AddError(
					"Task Wait Failed",
					fmt.Sprintf("Failed waiting for pool deletion task: %s", err),
				)
				return
			}
		}
	}
}

func (r *PoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *PoolResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.PoolType.IsNull() || data.PoolType.IsUnknown() {
		return
	}

	poolType := data.PoolType.ValueString()

	if poolType == "replicated" {
		if !data.ErasureCodeProfile.IsNull() && !data.ErasureCodeProfile.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("erasure_code_profile"),
				"Invalid Attribute Combination",
				"erasure_code_profile is only valid for erasure pools, not replicated pools.",
			)
		}
	}

	if poolType == "erasure" {
		if !data.Size.IsNull() && !data.Size.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("size"),
				"Invalid Attribute Combination",
				"size is only valid for replicated pools, not erasure pools.",
			)
		}

		if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root("min_size"),
				"Invalid Attribute Combination",
				"min_size is only valid for replicated pools, not erasure pools.",
			)
		}
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		if data.CompressionMode.ValueString() == "none" {
			if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("compression_algorithm"),
					"Invalid Attribute Combination",
					`compression_algorithm cannot be set when compression_mode is "none". Compression attributes are only valid when compression is enabled.`,
				)
			}

			if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("compression_required_ratio"),
					"Invalid Attribute Combination",
					`compression_required_ratio cannot be set when compression_mode is "none". Compression attributes are only valid when compression is enabled.`,
				)
			}

			if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("compression_min_blob_size"),
					"Invalid Attribute Combination",
					`compression_min_blob_size cannot be set when compression_mode is "none". Compression attributes are only valid when compression is enabled.`,
				)
			}

			if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
				resp.Diagnostics.AddAttributeError(
					path.Root("compression_max_blob_size"),
					"Invalid Attribute Combination",
					`compression_max_blob_size cannot be set when compression_mode is "none". Compression attributes are only valid when compression is enabled.`,
				)
			}
		}
	}

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		pgNum := data.PgNum.ValueInt64()
		if pgNum > 0 && bits.OnesCount64(uint64(pgNum)) != 1 {
			resp.Diagnostics.AddAttributeWarning(
				path.Root("pg_num"),
				"Non-Power-of-2 Placement Group Count",
				fmt.Sprintf("pg_num value %d is not a power of 2, which may cause suboptimal data distribution and generate a HEALTH_WARN in Ceph.", pgNum),
			)
		}
	}

	if data.PgNum.IsNull() &&
		!data.PgAutoscaleMode.IsUnknown() &&
		(data.PgAutoscaleMode.IsNull() || data.PgAutoscaleMode.ValueString() != "on") {
		resp.Diagnostics.AddAttributeError(
			path.Root("pg_num"),
			"Invalid Attribute Combination",
			`Either pg_num must be set or pg_autoscale_mode must be "on" so Ceph can size the pool's placement groups.`,
		)
	}

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() &&
		!data.PgAutoscaleMode.IsUnknown() && data.PgAutoscaleMode.ValueString() == "on" {
		resp.Diagnostics.AddAttributeError(
			path.Root("pg_num"),
			"Invalid Attribute Combination",
			`pg_num cannot be set when pg_autoscale_mode is "on"; the autoscaler owns the placement group count. Remove pg_num, or set pg_autoscale_mode to "off" or "warn" to manage it manually.`,
		)
	}
}
