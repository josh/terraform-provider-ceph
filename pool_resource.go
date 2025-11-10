package main

import (
	"context"
	"fmt"
	"math/bits"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &PoolResource{}
	_ resource.ResourceWithImportState    = &PoolResource{}
	_ resource.ResourceWithValidateConfig = &PoolResource{}
)

const (
	maxPoolReadRetries   = 10
	poolReadRetryDelay   = 500 * time.Millisecond
	poolUpdateSettleTime = 5 * time.Second
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
	MinSize                  types.Int64   `tfsdk:"min_size"`
	Size                     types.Int64   `tfsdk:"size"`
	PgAutoscaleMode          types.String  `tfsdk:"pg_autoscale_mode"`
	QuotaMaxObjects          types.Int64   `tfsdk:"quota_max_objects"`
	QuotaMaxBytes            types.Int64   `tfsdk:"quota_max_bytes"`
	CompressionMode          types.String  `tfsdk:"compression_mode"`
	CompressionAlgorithm     types.String  `tfsdk:"compression_algorithm"`
	CompressionRequiredRatio types.Float64 `tfsdk:"compression_required_ratio"`
	CompressionMinBlobSize   types.Int64   `tfsdk:"compression_min_blob_size"`
	CompressionMaxBlobSize   types.Int64   `tfsdk:"compression_max_blob_size"`
	PoolID                   types.Int64   `tfsdk:"pool_id"`
	PrimaryAffinity          types.Float64 `tfsdk:"primary_affinity"`
	ApplicationMetadata      types.List    `tfsdk:"application_metadata"`
	Flags                    types.Int64   `tfsdk:"flags"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"pg_autoscale_mode": resourceSchema.StringAttribute{
				MarkdownDescription: "The placement group autoscale mode. Must be one of: 'off', 'warn', or 'on'.",
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
			"pool_id": resourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the pool.",
				Computed:            true,
			},
			"primary_affinity": resourceSchema.Float64Attribute{
				MarkdownDescription: "The primary affinity of the pool.",
				Computed:            true,
			},
			"application_metadata": resourceSchema.ListAttribute{
				MarkdownDescription: "List of application types for the pool (e.g., [\"rbd\", \"rgw\"]).",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"flags": resourceSchema.Int64Attribute{
				MarkdownDescription: "The flags of the pool.",
				Computed:            true,
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

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		if data.PgAutoscaleMode.ValueString() == "on" {
			if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
				resp.Diagnostics.AddError(
					"Invalid Attribute Combination",
					"pg_num cannot be set when pg_autoscale_mode is 'on'. When autoscaling is enabled, Ceph automatically manages the number of placement groups.",
				)
				return
			}
			if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
				resp.Diagnostics.AddError(
					"Invalid Attribute Combination",
					"pgp_num cannot be set when pg_autoscale_mode is 'on'. When autoscaling is enabled, Ceph automatically manages the number of placement groups.",
				)
				return
			}
		}
	}

	createReq := r.buildCreateRequest(ctx, &data)

	err := r.client.CreatePool(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create pool: %s", err),
		)
		return
	}

	poolName := data.Name.ValueString()
	_, err = r.waitForPoolReadable(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read Pool After Creation",
			fmt.Sprintf("Failed to read pool %s: %s", poolName, err),
		)
		return
	}

	updateReq := CephAPIPoolUpdateRequest{}
	needsUpdate := false

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		val := data.PgAutoscaleMode.ValueString()
		updateReq.PgAutoscaleMode = &val
		needsUpdate = true
	}

	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		val := data.CompressionMode.ValueString()
		updateReq.CompressionMode = &val
		needsUpdate = true
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		val := data.CompressionAlgorithm.ValueString()
		updateReq.CompressionAlgorithm = &val
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
		err = r.client.UpdatePool(ctx, poolName, updateReq)
		if err != nil {
			resp.Diagnostics.AddError(
				"API Request Error",
				fmt.Sprintf("Pool was created but unable to set properties: %s", err),
			)
			return
		}

		timer := time.NewTimer(poolUpdateSettleTime)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			resp.Diagnostics.AddError(
				"Context Cancelled",
				"Pool creation was cancelled after update",
			)
			return
		}
	}

	pool, err := r.waitForPoolPropertiesAfterCreate(ctx, poolName, &data)
	if err != nil {
		resp.Diagnostics.AddError(
			"Pool Properties Verification Failed",
			fmt.Sprintf("Pool %s was created but properties did not converge: %s", poolName, err),
		)
		return
	}

	r.updateModelFromAPI(ctx, &data, pool, &resp.Diagnostics)

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

	r.updateModelFromAPI(ctx, &data, pool, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PoolResourceModel
	var state PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		if data.PgAutoscaleMode.ValueString() == "on" {
			if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
				resp.Diagnostics.AddError(
					"Invalid Attribute Combination",
					"pg_num cannot be set when pg_autoscale_mode is 'on'. When autoscaling is enabled, Ceph automatically manages the number of placement groups.",
				)
				return
			}
			if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
				resp.Diagnostics.AddError(
					"Invalid Attribute Combination",
					"pgp_num cannot be set when pg_autoscale_mode is 'on'. When autoscaling is enabled, Ceph automatically manages the number of placement groups.",
				)
				return
			}
		}
	}

	updateReq := r.buildUpdateRequest(ctx, &data)

	poolName := state.Name.ValueString()
	if !data.Name.Equal(state.Name) {
		newName := data.Name.ValueString()
		updateReq.Pool = &newName
	}

	err := r.waitForPoolStateStable(ctx, poolName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Pool State Stability Check Failed",
			fmt.Sprintf("Unable to verify pool state is stable before update: %s", err),
		)
		return
	}

	err = r.client.UpdatePool(ctx, poolName, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to update pool: %s", err),
		)
		return
	}

	if updateReq.Pool != nil {
		poolName = *updateReq.Pool
	}

	timer := time.NewTimer(poolUpdateSettleTime)
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

	pool, err := r.waitForPoolPropertiesAfterUpdate(ctx, poolName, &data)
	if err != nil {
		resp.Diagnostics.AddError(
			"Pool Properties Verification Failed",
			fmt.Sprintf("Pool %s was updated but properties did not converge: %s", poolName, err),
		)
		return
	}

	r.updateModelFromAPI(ctx, &data, pool, &resp.Diagnostics)

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

	var data PoolResourceModel
	data.Name = types.StringValue(poolName)

	r.updateModelFromAPI(ctx, &data, pool, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
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

}

func (r *PoolResource) buildCreateRequest(ctx context.Context, data *PoolResourceModel) CephAPIPoolCreateRequest {
	poolType := data.PoolType.ValueString()
	createReq := CephAPIPoolCreateRequest{
		Pool:     data.Name.ValueString(),
		PoolType: &poolType,
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
		val := data.CrushRule.ValueString()
		createReq.CrushRule = &val
	}
	if !data.ErasureCodeProfile.IsNull() && !data.ErasureCodeProfile.IsUnknown() {
		val := data.ErasureCodeProfile.ValueString()
		createReq.ErasureCodeProfile = &val
	}
	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		var apps []string
		diags := data.ApplicationMetadata.ElementsAs(ctx, &apps, false)
		if !diags.HasError() {
			createReq.ApplicationMetadata = apps
		}
	}
	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		minSize := int(data.MinSize.ValueInt64())
		createReq.MinSize = &minSize
	}
	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		size := int(data.Size.ValueInt64())
		createReq.Size = &size
	}
	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		val := data.PgAutoscaleMode.ValueString()
		createReq.PgAutoscaleMode = &val
	}
	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		quotaMaxObjects := int(data.QuotaMaxObjects.ValueInt64())
		createReq.QuotaMaxObjects = &quotaMaxObjects
	}
	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		quotaMaxBytes := int(data.QuotaMaxBytes.ValueInt64())
		createReq.QuotaMaxBytes = &quotaMaxBytes
	}
	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		val := data.CompressionMode.ValueString()
		createReq.CompressionMode = &val
	}
	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		val := data.CompressionAlgorithm.ValueString()
		createReq.CompressionAlgorithm = &val
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

	return createReq
}

func (r *PoolResource) buildUpdateRequest(ctx context.Context, data *PoolResourceModel) CephAPIPoolUpdateRequest {
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
		val := data.CrushRule.ValueString()
		updateReq.CrushRule = &val
	}
	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		size := int(data.Size.ValueInt64())
		updateReq.Size = &size
	}
	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		minSize := int(data.MinSize.ValueInt64())
		updateReq.MinSize = &minSize
	}
	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		val := data.PgAutoscaleMode.ValueString()
		updateReq.PgAutoscaleMode = &val
	}
	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		quotaMaxObjects := int(data.QuotaMaxObjects.ValueInt64())
		updateReq.QuotaMaxObjects = &quotaMaxObjects
	}
	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		quotaMaxBytes := int(data.QuotaMaxBytes.ValueInt64())
		updateReq.QuotaMaxBytes = &quotaMaxBytes
	}
	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		val := data.CompressionMode.ValueString()
		updateReq.CompressionMode = &val
	}
	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		val := data.CompressionAlgorithm.ValueString()
		updateReq.CompressionAlgorithm = &val
	}
	if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
		compressionRequiredRatio := data.CompressionRequiredRatio.ValueFloat64()
		updateReq.CompressionRequiredRatio = &compressionRequiredRatio
	}
	if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
		compressionMinBlobSize := int(data.CompressionMinBlobSize.ValueInt64())
		updateReq.CompressionMinBlobSize = &compressionMinBlobSize
	}
	if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
		compressionMaxBlobSize := int(data.CompressionMaxBlobSize.ValueInt64())
		updateReq.CompressionMaxBlobSize = &compressionMaxBlobSize
	}
	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		var apps []string
		diags := data.ApplicationMetadata.ElementsAs(ctx, &apps, false)
		if !diags.HasError() {
			updateReq.ApplicationMetadata = apps
		}
	}

	return updateReq
}

func (r *PoolResource) waitForPoolReadable(ctx context.Context, poolName string) (*CephAPIPool, error) {
	var pool *CephAPIPool
	var err error

	for i := range maxPoolReadRetries {
		pool, err = r.client.GetPool(ctx, poolName)
		if err == nil {
			return pool, nil
		}

		if strings.Contains(err.Error(), "404") && i < maxPoolReadRetries-1 {
			select {
			case <-time.After(poolReadRetryDelay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, err
}

func (r *PoolResource) waitForPoolStateStable(ctx context.Context, poolName string) error {
	const maxStabilityRetries = 5
	const stabilityCheckDelay = 500 * time.Millisecond

	for i := 0; i < maxStabilityRetries; i++ {
		pool1, err1 := r.client.GetPool(ctx, poolName)
		if err1 != nil {
			return fmt.Errorf("unable to read pool for stability check: %w", err1)
		}

		select {
		case <-time.After(stabilityCheckDelay):
		case <-ctx.Done():
			return ctx.Err()
		}

		pool2, err2 := r.client.GetPool(ctx, poolName)
		if err2 != nil {
			return fmt.Errorf("unable to read pool for stability check: %w", err2)
		}

		if areApplicationsEqual(pool1.ApplicationMetadata, pool2.ApplicationMetadata) {
			return nil
		}

		if i < maxStabilityRetries-1 {
			select {
			case <-time.After(stabilityCheckDelay):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

func areApplicationsEqual(apps1, apps2 []string) bool {
	if len(apps1) != len(apps2) {
		return false
	}

	set1 := make(map[string]bool, len(apps1))
	for _, app := range apps1 {
		set1[app] = true
	}

	for _, app := range apps2 {
		if !set1[app] {
			return false
		}
	}

	return true
}

func (r *PoolResource) waitForPoolPropertiesAfterCreate(ctx context.Context, poolName string, data *PoolResourceModel) (*CephAPIPool, error) {
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

	var expectedApplications []string
	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		data.ApplicationMetadata.ElementsAs(ctx, &expectedApplications, false)
	}

	expectedQuotaMaxObjects := int64(-1)
	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		expectedQuotaMaxObjects = data.QuotaMaxObjects.ValueInt64()
	}

	expectedQuotaMaxBytes := int64(-1)
	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		expectedQuotaMaxBytes = data.QuotaMaxBytes.ValueInt64()
	}

	var pool *CephAPIPool
	var err error

	for i := range maxPoolReadRetries {
		pool, err = r.client.GetPool(ctx, poolName)
		if err != nil {
			if strings.Contains(err.Error(), "404") && i < maxPoolReadRetries-1 {
				select {
				case <-time.After(poolReadRetryDelay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		sizeMatches := (expectedSize == 0) || (pool.Size == expectedSize)
		compressionModeMatches := (expectedCompressionMode == "") || (pool.Options.CompressionMode == expectedCompressionMode)
		compressionAlgorithmMatches := (expectedCompressionAlgorithm == "") || (pool.Options.CompressionAlgorithm == expectedCompressionAlgorithm)

		applicationMatches := true
		if len(expectedApplications) > 0 {
			expectedSet := make(map[string]bool)
			for _, app := range expectedApplications {
				expectedSet[app] = true
			}
			actualSet := make(map[string]bool)
			for _, app := range pool.ApplicationMetadata {
				actualSet[app] = true
			}
			if len(expectedSet) != len(actualSet) {
				applicationMatches = false
			} else {
				for app := range expectedSet {
					if !actualSet[app] {
						applicationMatches = false
						break
					}
				}
			}
		}

		quotaMaxObjectsMatches := (expectedQuotaMaxObjects == -1) || (int64(pool.QuotaMaxObjects) == expectedQuotaMaxObjects)
		quotaMaxBytesMatches := (expectedQuotaMaxBytes == -1) || (int64(pool.QuotaMaxBytes) == expectedQuotaMaxBytes)

		if sizeMatches && compressionModeMatches && compressionAlgorithmMatches && applicationMatches && quotaMaxObjectsMatches && quotaMaxBytesMatches {
			return pool, nil
		}

		if i < maxPoolReadRetries-1 {
			select {
			case <-time.After(poolReadRetryDelay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	var mismatches []string
	if pool != nil {
		if expectedSize != 0 && pool.Size != expectedSize {
			mismatches = append(mismatches, fmt.Sprintf("size (expected %d, got %d)", expectedSize, pool.Size))
		}
		if expectedCompressionMode != "" && pool.Options.CompressionMode != expectedCompressionMode {
			mismatches = append(mismatches, fmt.Sprintf("compression_mode (expected %q, got %q)", expectedCompressionMode, pool.Options.CompressionMode))
		}
		if expectedCompressionAlgorithm != "" && pool.Options.CompressionAlgorithm != expectedCompressionAlgorithm {
			mismatches = append(mismatches, fmt.Sprintf("compression_algorithm (expected %q, got %q)", expectedCompressionAlgorithm, pool.Options.CompressionAlgorithm))
		}
		if len(expectedApplications) > 0 {
			expectedSet := make(map[string]bool)
			for _, app := range expectedApplications {
				expectedSet[app] = true
			}
			actualSet := make(map[string]bool)
			for _, app := range pool.ApplicationMetadata {
				actualSet[app] = true
			}
			if len(expectedSet) != len(actualSet) || !func() bool {
				for app := range expectedSet {
					if !actualSet[app] {
						return false
					}
				}
				return true
			}() {
				mismatches = append(mismatches, fmt.Sprintf("application_metadata (expected %v, got %v)", expectedApplications, pool.ApplicationMetadata))
			}
		}
		if expectedQuotaMaxObjects != -1 && int64(pool.QuotaMaxObjects) != expectedQuotaMaxObjects {
			mismatches = append(mismatches, fmt.Sprintf("quota_max_objects (expected %d, got %d)", expectedQuotaMaxObjects, pool.QuotaMaxObjects))
		}
		if expectedQuotaMaxBytes != -1 && int64(pool.QuotaMaxBytes) != expectedQuotaMaxBytes {
			mismatches = append(mismatches, fmt.Sprintf("quota_max_bytes (expected %d, got %d)", expectedQuotaMaxBytes, pool.QuotaMaxBytes))
		}
	}

	return pool, fmt.Errorf("properties did not converge after %d retries: %s",
		maxPoolReadRetries, strings.Join(mismatches, ", "))
}

func (r *PoolResource) waitForPoolPropertiesAfterUpdate(ctx context.Context, poolName string, data *PoolResourceModel) (*CephAPIPool, error) {
	expectedPgAutoscaleMode := ""
	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		expectedPgAutoscaleMode = data.PgAutoscaleMode.ValueString()
	}

	expectedCompressionMode := ""
	if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
		expectedCompressionMode = data.CompressionMode.ValueString()
	}

	expectedCompressionAlgorithm := ""
	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		expectedCompressionAlgorithm = data.CompressionAlgorithm.ValueString()
	}

	expectedPgNum := 0
	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		expectedPgNum = int(data.PgNum.ValueInt64())
	}

	expectedPgpNum := 0
	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
		expectedPgpNum = int(data.PgpNum.ValueInt64())
	}

	expectedQuotaMaxObjects := int64(-1)
	if !data.QuotaMaxObjects.IsNull() && !data.QuotaMaxObjects.IsUnknown() {
		expectedQuotaMaxObjects = data.QuotaMaxObjects.ValueInt64()
	}

	expectedQuotaMaxBytes := int64(-1)
	if !data.QuotaMaxBytes.IsNull() && !data.QuotaMaxBytes.IsUnknown() {
		expectedQuotaMaxBytes = data.QuotaMaxBytes.ValueInt64()
	}

	expectedSize := 0
	if !data.Size.IsNull() && !data.Size.IsUnknown() {
		expectedSize = int(data.Size.ValueInt64())
	}

	expectedMinSize := 0
	if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
		expectedMinSize = int(data.MinSize.ValueInt64())
	}

	var expectedApplications []string
	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		data.ApplicationMetadata.ElementsAs(ctx, &expectedApplications, false)
	}

	var pool *CephAPIPool
	var err error

	for i := range maxPoolReadRetries {
		pool, err = r.client.GetPool(ctx, poolName)
		if err != nil {
			if strings.Contains(err.Error(), "404") && i < maxPoolReadRetries-1 {
				select {
				case <-time.After(poolReadRetryDelay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return nil, err
		}

		pgAutoscaleModeMatches := (expectedPgAutoscaleMode == "") || (pool.PGAutoscaleMode == expectedPgAutoscaleMode)
		compressionModeMatches := (expectedCompressionMode == "") || (pool.Options.CompressionMode == expectedCompressionMode)
		compressionAlgorithmMatches := (expectedCompressionAlgorithm == "") || (pool.Options.CompressionAlgorithm == expectedCompressionAlgorithm)
		pgNumMatches := (expectedPgNum == 0) || (pool.PGNum == expectedPgNum)
		pgpNumMatches := (expectedPgpNum == 0) || (pool.PGPlacementNum == expectedPgpNum)
		quotaMaxObjectsMatches := (expectedQuotaMaxObjects == -1) || (int64(pool.QuotaMaxObjects) == expectedQuotaMaxObjects)
		quotaMaxBytesMatches := (expectedQuotaMaxBytes == -1) || (int64(pool.QuotaMaxBytes) == expectedQuotaMaxBytes)
		sizeMatches := (expectedSize == 0) || (pool.Size == expectedSize)
		minSizeMatches := (expectedMinSize == 0) || (pool.MinSize == expectedMinSize)

		applicationMatches := true
		if len(expectedApplications) > 0 {
			expectedSet := make(map[string]bool)
			for _, app := range expectedApplications {
				expectedSet[app] = true
			}
			actualSet := make(map[string]bool)
			for _, app := range pool.ApplicationMetadata {
				actualSet[app] = true
			}
			if len(expectedSet) != len(actualSet) {
				applicationMatches = false
			} else {
				for app := range expectedSet {
					if !actualSet[app] {
						applicationMatches = false
						break
					}
				}
			}
		}

		if pgAutoscaleModeMatches && compressionModeMatches && compressionAlgorithmMatches && pgNumMatches && pgpNumMatches && quotaMaxObjectsMatches && quotaMaxBytesMatches && sizeMatches && minSizeMatches && applicationMatches {
			return pool, nil
		}

		if i < maxPoolReadRetries-1 {
			select {
			case <-time.After(poolReadRetryDelay):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	var mismatches []string
	if pool != nil {
		if expectedPgAutoscaleMode != "" && pool.PGAutoscaleMode != expectedPgAutoscaleMode {
			mismatches = append(mismatches, fmt.Sprintf("pg_autoscale_mode (expected %q, got %q)", expectedPgAutoscaleMode, pool.PGAutoscaleMode))
		}
		if expectedCompressionMode != "" && pool.Options.CompressionMode != expectedCompressionMode {
			mismatches = append(mismatches, fmt.Sprintf("compression_mode (expected %q, got %q)", expectedCompressionMode, pool.Options.CompressionMode))
		}
		if expectedCompressionAlgorithm != "" && pool.Options.CompressionAlgorithm != expectedCompressionAlgorithm {
			mismatches = append(mismatches, fmt.Sprintf("compression_algorithm (expected %q, got %q)", expectedCompressionAlgorithm, pool.Options.CompressionAlgorithm))
		}
		if expectedPgNum != 0 && pool.PGNum != expectedPgNum {
			mismatches = append(mismatches, fmt.Sprintf("pg_num (expected %d, got %d)", expectedPgNum, pool.PGNum))
		}
		if expectedPgpNum != 0 && pool.PGPlacementNum != expectedPgpNum {
			mismatches = append(mismatches, fmt.Sprintf("pgp_num (expected %d, got %d)", expectedPgpNum, pool.PGPlacementNum))
		}
		if expectedQuotaMaxObjects != -1 && int64(pool.QuotaMaxObjects) != expectedQuotaMaxObjects {
			mismatches = append(mismatches, fmt.Sprintf("quota_max_objects (expected %d, got %d)", expectedQuotaMaxObjects, pool.QuotaMaxObjects))
		}
		if expectedQuotaMaxBytes != -1 && int64(pool.QuotaMaxBytes) != expectedQuotaMaxBytes {
			mismatches = append(mismatches, fmt.Sprintf("quota_max_bytes (expected %d, got %d)", expectedQuotaMaxBytes, pool.QuotaMaxBytes))
		}
		if expectedSize != 0 && pool.Size != expectedSize {
			mismatches = append(mismatches, fmt.Sprintf("size (expected %d, got %d)", expectedSize, pool.Size))
		}
		if expectedMinSize != 0 && pool.MinSize != expectedMinSize {
			mismatches = append(mismatches, fmt.Sprintf("min_size (expected %d, got %d)", expectedMinSize, pool.MinSize))
		}
		if len(expectedApplications) > 0 {
			expectedSet := make(map[string]bool)
			for _, app := range expectedApplications {
				expectedSet[app] = true
			}
			actualSet := make(map[string]bool)
			for _, app := range pool.ApplicationMetadata {
				actualSet[app] = true
			}
			mismatch := len(expectedSet) != len(actualSet)
			if !mismatch {
				for app := range expectedSet {
					if !actualSet[app] {
						mismatch = true
						break
					}
				}
			}
			if mismatch {
				mismatches = append(mismatches, fmt.Sprintf("application_metadata (expected %v, got %v)", expectedApplications, pool.ApplicationMetadata))
			}
		}
	}

	return pool, fmt.Errorf("properties did not converge after %d retries: %s",
		maxPoolReadRetries, strings.Join(mismatches, ", "))
}

func (r *PoolResource) updateModelFromAPI(ctx context.Context, data *PoolResourceModel, pool *CephAPIPool, diagnostics *diag.Diagnostics) {
	data.PoolID = types.Int64Value(int64(pool.PoolID))
	data.PoolType = types.StringValue(pool.Type)
	data.Size = types.Int64Value(int64(pool.Size))
	data.MinSize = types.Int64Value(int64(pool.MinSize))

	autoscaleMode := pool.PGAutoscaleMode

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

	data.CrushRule = types.StringValue(pool.CrushRule)
	data.PrimaryAffinity = types.Float64Value(pool.PrimaryAffinity)

	appMeta, diags := types.ListValueFrom(ctx, types.StringType, pool.ApplicationMetadata)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}
	data.ApplicationMetadata = appMeta

	data.ErasureCodeProfile = types.StringValue(pool.ErasureCodeProfile)
	data.PgAutoscaleMode = types.StringValue(pool.PGAutoscaleMode)
	data.QuotaMaxObjects = types.Int64Value(int64(pool.QuotaMaxObjects))
	data.QuotaMaxBytes = types.Int64Value(int64(pool.QuotaMaxBytes))
	data.CompressionMode = types.StringValue(pool.Options.CompressionMode)
	data.CompressionAlgorithm = types.StringValue(pool.Options.CompressionAlgorithm)
	data.CompressionRequiredRatio = types.Float64Value(pool.Options.CompressionRequiredRatio)
	data.CompressionMinBlobSize = types.Int64Value(int64(pool.Options.CompressionMinBlobSize))
	data.CompressionMaxBlobSize = types.Int64Value(int64(pool.Options.CompressionMaxBlobSize))
	data.Flags = types.Int64Value(int64(pool.Flags))
}
