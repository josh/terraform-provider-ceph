package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &PoolResource{}
	_ resource.ResourceWithImportState = &PoolResource{}
)

func newPoolResource() resource.Resource {
	return &PoolResource{}
}

type PoolResource struct {
	client *CephAPIClient
}

type PoolResourceModel struct {
	Name                     types.String  `tfsdk:"name"`
	PoolType                 types.String  `tfsdk:"pool_type"`
	PgNum                    types.Int64   `tfsdk:"pg_num"`
	PgpNum                   types.Int64   `tfsdk:"pgp_num"`
	CrushRule                types.String  `tfsdk:"crush_rule"`
	ErasureCodeProfile       types.String  `tfsdk:"erasure_code_profile"`
	Application              types.String  `tfsdk:"application"`
	MinSize                  types.Int64   `tfsdk:"min_size"`
	Size                     types.Int64   `tfsdk:"size"`
	AutoscaleMode            types.String  `tfsdk:"autoscale_mode"`
	PgAutoscaleMode          types.String  `tfsdk:"pg_autoscale_mode"`
	TargetSizeRatio          types.Float64 `tfsdk:"target_size_ratio"`
	TargetSizeBytes          types.Int64   `tfsdk:"target_size_bytes"`
	CompressionMode          types.String  `tfsdk:"compression_mode"`
	CompressionAlgorithm     types.String  `tfsdk:"compression_algorithm"`
	CompressionRequiredRatio types.Float64 `tfsdk:"compression_required_ratio"`
	CompressionMinBlobSize   types.Int64   `tfsdk:"compression_min_blob_size"`
	CompressionMaxBlobSize   types.Int64   `tfsdk:"compression_max_blob_size"`
	PoolID                   types.Int64   `tfsdk:"pool_id"`
	PGPlacementNum           types.Int64   `tfsdk:"pg_placement_num"`
	CrashReplayInterval      types.Int64   `tfsdk:"crash_replay_interval"`
	PrimaryAffinity          types.Float64 `tfsdk:"primary_affinity"`
	ApplicationMetadata      types.Dynamic `tfsdk:"application_metadata"`
	Flags                    types.Int64   `tfsdk:"flags"`
	TargetSizeRatioRel       types.Float64 `tfsdk:"target_size_ratio_rel"`
	MinPGNum                 types.Int64   `tfsdk:"min_pg_num"`
	PGAutoscalerProfile      types.String  `tfsdk:"pg_autoscaler_profile"`
	Configuration            types.List    `tfsdk:"configuration"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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
				MarkdownDescription: "The number of placement groups for the pool.",
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
			"application": resourceSchema.StringAttribute{
				MarkdownDescription: "The application type for the pool (e.g., rbd, cephfs, rgw).",
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
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of replicas for the pool.",
				Optional:            true,
				Computed:            true,
			},
			"autoscale_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The autoscale mode of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pg_autoscale_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The placement group autoscale mode. Must be one of: 'off', 'warn', or 'on'.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("off", "warn", "on"),
				},
			},
			"target_size_ratio": resourceSchema.Float64Attribute{
				MarkdownDescription: "The target size ratio of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
					float64planmodifier.UseStateForUnknown(),
				},
			},
			"target_size_bytes": resourceSchema.Int64Attribute{
				MarkdownDescription: "The target size in bytes of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"compression_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The compression mode of the pool. Must be one of: 'none', 'passive', 'aggressive', or 'force'.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("none", "passive", "aggressive", "force"),
				},
			},
			"compression_algorithm": resourceSchema.StringAttribute{
				MarkdownDescription: "The compression algorithm of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"compression_required_ratio": resourceSchema.Float64Attribute{
				MarkdownDescription: "The compression required ratio of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Float64{
					float64planmodifier.RequiresReplace(),
					float64planmodifier.UseStateForUnknown(),
				},
			},
			"compression_min_blob_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The compression minimum blob size of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"compression_max_blob_size": resourceSchema.Int64Attribute{
				MarkdownDescription: "The compression maximum blob size of the pool.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"pool_id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the pool.",
				Computed:            true,
			},
			"pg_placement_num": resourceSchema.Int64Attribute{
				MarkdownDescription: "The number of placement groups for placement (computed).",
				Computed:            true,
			},
			"crash_replay_interval": resourceSchema.Int64Attribute{
				MarkdownDescription: "The crash replay interval in seconds.",
				Computed:            true,
			},
			"primary_affinity": resourceSchema.Float64Attribute{
				MarkdownDescription: "The primary affinity of the pool.",
				Computed:            true,
			},
			"application_metadata": resourceSchema.DynamicAttribute{
				MarkdownDescription: "The application metadata of the pool.",
				Computed:            true,
			},
			"flags": resourceSchema.Int64Attribute{
				MarkdownDescription: "The flags of the pool.",
				Computed:            true,
			},
			"target_size_ratio_rel": resourceSchema.Float64Attribute{
				MarkdownDescription: "The target size ratio relative to the cluster.",
				Computed:            true,
			},
			"min_pg_num": resourceSchema.Int64Attribute{
				MarkdownDescription: "The minimum number of placement groups for the pool.",
				Computed:            true,
			},
			"pg_autoscaler_profile": resourceSchema.StringAttribute{
				MarkdownDescription: "The placement group autoscaler profile.",
				Computed:            true,
			},
			"configuration": resourceSchema.ListAttribute{
				MarkdownDescription: "The configuration of the pool.",
				Computed:            true,
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"name":  types.StringType,
						"value": types.StringType,
					},
				},
			},
		},
	}
}

func (r *PoolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *PoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createReq := CephAPIPoolCreateRequest{
		Pool:     data.Name.ValueString(),
		PoolType: data.PoolType.ValueString(),
	}

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		pgNum := int(data.PgNum.ValueInt64())
		createReq.PgNum = &pgNum
	}

	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
		pgpNum := int(data.PgpNum.ValueInt64())
		createReq.PgpNum = &pgpNum
	}

	if !data.CrushRule.IsNull() && !data.CrushRule.IsUnknown() {
		createReq.CrushRule = data.CrushRule.ValueString()
	}

	if !data.ErasureCodeProfile.IsNull() && !data.ErasureCodeProfile.IsUnknown() {
		createReq.ErasureCodeProfile = data.ErasureCodeProfile.ValueString()
	}

	if !data.Application.IsNull() && !data.Application.IsUnknown() {
		createReq.ApplicationMetadata = []string{data.Application.ValueString()}
	}

	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		minSize := int(data.MinSize.ValueInt64())
		createReq.MinSize = &minSize
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		size := int(data.Size.ValueInt64())
		createReq.Size = &size
	}

	if !data.AutoscaleMode.IsNull() && !data.AutoscaleMode.IsUnknown() {
		createReq.AutoscaleMode = data.AutoscaleMode.ValueString()
	}

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		createReq.PgAutoscaleMode = data.PgAutoscaleMode.ValueString()
	}

	if !data.TargetSizeRatio.IsNull() && !data.TargetSizeRatio.IsUnknown() {
		targetSizeRatio := data.TargetSizeRatio.ValueFloat64()
		createReq.TargetSizeRatio = &targetSizeRatio
	}

	if !data.TargetSizeBytes.IsNull() && !data.TargetSizeBytes.IsUnknown() {
		targetSizeBytes := int(data.TargetSizeBytes.ValueInt64())
		createReq.TargetSizeBytes = &targetSizeBytes
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		createReq.CompressionMode = data.CompressionMode.ValueString()
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		createReq.CompressionAlgorithm = data.CompressionAlgorithm.ValueString()
	}

	if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
		compressionRequiredRatio := data.CompressionRequiredRatio.ValueFloat64()
		createReq.CompressionRequiredRatio = &compressionRequiredRatio
	}

	if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
		compressionMinBlobSize := int(data.CompressionMinBlobSize.ValueInt64())
		createReq.CompressionMinBlobSize = &compressionMinBlobSize
	}

	if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
		compressionMaxBlobSize := int(data.CompressionMaxBlobSize.ValueInt64())
		createReq.CompressionMaxBlobSize = &compressionMaxBlobSize
	}

	err := r.client.CreatePool(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create pool: %s", err),
		)
		return
	}

	updateReq := CephAPIPoolUpdateRequest{}
	needsUpdate := false

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		updateReq.PgAutoscaleMode = data.PgAutoscaleMode.ValueString()
		needsUpdate = true
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		updateReq.CompressionMode = data.CompressionMode.ValueString()
		needsUpdate = true
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		updateReq.CompressionAlgorithm = data.CompressionAlgorithm.ValueString()
		needsUpdate = true
	}

	if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
		compressionRequiredRatio := data.CompressionRequiredRatio.ValueFloat64()
		updateReq.CompressionRequiredRatio = &compressionRequiredRatio
		needsUpdate = true
	}

	if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
		compressionMinBlobSize := int(data.CompressionMinBlobSize.ValueInt64())
		updateReq.CompressionMinBlobSize = &compressionMinBlobSize
		needsUpdate = true
	}

	if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
		compressionMaxBlobSize := int(data.CompressionMaxBlobSize.ValueInt64())
		updateReq.CompressionMaxBlobSize = &compressionMaxBlobSize
		needsUpdate = true
	}

	if needsUpdate {
		err = r.client.UpdatePool(ctx, data.Name.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Pool was created but unable to set properties: %s", err),
			)
			return
		}
	}

	var pool *CephAPIPool
	maxRetries := 10
	expectedSize := 0
	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		expectedSize = int(data.Size.ValueInt64())
	}

	expectedCompressionMode := ""
	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		expectedCompressionMode = data.CompressionMode.ValueString()
	}

	expectedCompressionAlgorithm := ""
	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		expectedCompressionAlgorithm = data.CompressionAlgorithm.ValueString()
	}

	expectedApplication := ""
	if !data.Application.IsNull() && !data.Application.IsUnknown() {
		expectedApplication = data.Application.ValueString()
	}

	for i := 0; i < maxRetries; i++ {
		pool, err = r.client.GetPool(ctx, data.Name.ValueString())
		if err == nil {
			sizeMatches := (expectedSize == 0) || (pool.Size == expectedSize)
			compressionModeMatches := (expectedCompressionMode == "") || (pool.Options.CompressionMode == expectedCompressionMode)
			compressionAlgorithmMatches := (expectedCompressionAlgorithm == "") || (pool.Options.CompressionAlgorithm == expectedCompressionAlgorithm)

			application := pool.Application
			if application == "" && len(pool.ApplicationMetadata) > 0 {
				application = pool.ApplicationMetadata[0]
			}
			applicationMatches := (expectedApplication == "") || (application == expectedApplication)

			if sizeMatches && compressionModeMatches && compressionAlgorithmMatches && applicationMatches {
				break
			}

			if i < maxRetries-1 {
				select {
				case <-time.After(500 * time.Millisecond):
				case <-ctx.Done():
					resp.Diagnostics.AddError(
						"Context Cancelled",
						"Pool creation was cancelled",
					)
					return
				}
				continue
			}
		}

		if err != nil && strings.Contains(err.Error(), "404") && i < maxRetries-1 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				resp.Diagnostics.AddError(
					"Context Cancelled",
					"Pool creation was cancelled",
				)
				return
			}
			continue
		}

		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Read Pool After Creation",
				fmt.Sprintf("Failed to read pool %s: %s", data.Name.ValueString(), err),
			)
			return
		}
	}

	config, err := r.client.GetPoolConfiguration(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Pool Configuration After Creation",
			fmt.Sprintf("Failed to read pool configuration: %s", err),
		)
		return
	}

	r.updateModelFromAPI(ctx, &data, pool, config, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := r.client.GetPool(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool: %s", err),
		)
		return
	}

	config, err := r.client.GetPoolConfiguration(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool configuration: %s", err),
		)
		return
	}

	r.updateModelFromAPI(ctx, &data, pool, config, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := CephAPIPoolUpdateRequest{}

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		pgNum := int(data.PgNum.ValueInt64())
		updateReq.PgNum = &pgNum
	}

	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
		pgpNum := int(data.PgpNum.ValueInt64())
		updateReq.PgpNum = &pgpNum
	}

	if !data.CrushRule.IsNull() && !data.CrushRule.IsUnknown() {
		updateReq.CrushRule = data.CrushRule.ValueString()
	}

	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		size := int(data.Size.ValueInt64())
		updateReq.Size = &size
	}

	err := r.client.UpdatePool(ctx, data.Name.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update pool: %s", err),
		)
		return
	}

	timer := time.NewTimer(3 * time.Second)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		resp.Diagnostics.AddError(
			"Context Cancelled",
			"Update operation was cancelled before completion",
		)
		return
	}

	var pool *CephAPIPool
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		pool, err = r.client.GetPool(ctx, data.Name.ValueString())
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "404") && i < maxRetries-1 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		resp.Diagnostics.AddError(
			"Unable to Read Pool After Update",
			fmt.Sprintf("Failed to read pool: %s", err),
		)
		return
	}

	config, err := r.client.GetPoolConfiguration(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Pool Configuration After Update",
			fmt.Sprintf("Failed to read pool configuration: %s", err),
		)
		return
	}

	r.updateModelFromAPI(ctx, &data, pool, config, &resp.Diagnostics)

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

	err := r.client.DeletePool(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete pool: %s", err),
		)
		return
	}
}

func (r *PoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	poolName := req.ID

	pool, err := r.client.GetPool(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool during import: %s", err),
		)
		return
	}

	config, err := r.client.GetPoolConfiguration(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to read pool configuration during import: %s", err),
		)
		return
	}

	var data PoolResourceModel
	data.Name = types.StringValue(poolName)

	r.updateModelFromAPI(ctx, &data, pool, config, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) updateModelFromAPI(ctx context.Context, data *PoolResourceModel, pool *CephAPIPool, config CephAPIPoolConfiguration, diagnostics *diag.Diagnostics) {
	data.PoolID = types.Int64Value(int64(pool.PoolID))
	data.PoolType = types.StringValue(pool.Type)
	data.Size = types.Int64Value(int64(pool.Size))
	data.MinSize = types.Int64Value(int64(pool.MinSize))

	autoscaleMode := pool.PGAutoscaleMode
	if autoscaleMode == "" {
		autoscaleMode = pool.AutoscaleMode
	}

	if autoscaleMode == "off" || autoscaleMode == "warn" || autoscaleMode == "" {
		data.PgNum = types.Int64Value(int64(pool.PGNum))
		data.PgpNum = types.Int64Value(int64(pool.PGPlacementNum))
	} else {
		if data.PgNum.IsNull() || data.PgNum.IsUnknown() {
			data.PgNum = types.Int64Value(int64(pool.PGNum))
		}
		if data.PgpNum.IsNull() || data.PgpNum.IsUnknown() {
			data.PgpNum = types.Int64Value(int64(pool.PGPlacementNum))
		}
	}

	data.PGPlacementNum = types.Int64Value(int64(pool.PGPlacementNum))
	data.CrushRule = types.StringValue(pool.CrushRule)
	data.CrashReplayInterval = types.Int64Value(int64(pool.CrashReplayInterval))
	data.PrimaryAffinity = types.Float64Value(pool.PrimaryAffinity)

	application := pool.Application
	if application == "" && len(pool.ApplicationMetadata) > 0 {
		application = pool.ApplicationMetadata[0]
	}
	data.Application = types.StringValue(application)

	data.ErasureCodeProfile = types.StringValue(pool.ErasureCodeProfile)
	data.PgAutoscaleMode = types.StringValue(pool.PGAutoscaleMode)
	data.AutoscaleMode = types.StringValue(pool.AutoscaleMode)
	data.TargetSizeRatio = types.Float64Value(pool.Options.TargetSizeRatio)
	data.TargetSizeBytes = types.Int64Value(int64(pool.Options.TargetSizeBytes))
	data.TargetSizeRatioRel = types.Float64Value(pool.TargetSizeRatioRel)
	data.MinPGNum = types.Int64Value(int64(pool.MinPGNum))
	data.PGAutoscalerProfile = types.StringValue(pool.PGAutoscalerProfile)
	data.CompressionMode = types.StringValue(pool.Options.CompressionMode)
	data.CompressionAlgorithm = types.StringValue(pool.Options.CompressionAlgorithm)
	data.CompressionRequiredRatio = types.Float64Value(pool.Options.CompressionRequiredRatio)
	data.CompressionMinBlobSize = types.Int64Value(int64(pool.Options.CompressionMinBlobSize))
	data.CompressionMaxBlobSize = types.Int64Value(int64(pool.Options.CompressionMaxBlobSize))
	data.Flags = types.Int64Value(int64(pool.Flags))

	appMeta, diags := types.ListValueFrom(ctx, types.DynamicType, pool.ApplicationMetadata)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}
	data.ApplicationMetadata = types.DynamicValue(appMeta)

	configObjects := make([]attr.Value, 0, len(config))
	for _, item := range config {
		configMap := map[string]attr.Value{
			"name":  types.StringValue(item.Name),
			"value": types.StringValue(fmt.Sprintf("%v", item.Value)),
		}
		configObject, diags := types.ObjectValue(map[string]attr.Type{
			"name":  types.StringType,
			"value": types.StringType,
		}, configMap)
		diagnostics.Append(diags...)
		if diagnostics.HasError() {
			return
		}
		configObjects = append(configObjects, configObject)
	}

	conf, diags := types.ListValue(types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":  types.StringType,
		"value": types.StringType,
	}}, configObjects)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}
	data.Configuration = conf
}
