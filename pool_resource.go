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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &PoolResource{}
	_ resource.ResourceWithImportState    = &PoolResource{}
	_ resource.ResourceWithValidateConfig = &PoolResource{}
)

const (
	poolPropertiesTimeout     = 60 * time.Second
	poolPropertyCheckInterval = 500 * time.Millisecond
	poolRenameSettleTime      = 5 * time.Second
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
		Pool: data.Name.ValueString(),
	}

	poolType := data.PoolType.ValueString()
	createReq.PoolType = &poolType

	if !data.PgNum.IsNull() && !data.PgNum.IsUnknown() {
		pgNum := int(data.PgNum.ValueInt64())
		createReq.PgNum = &pgNum
	}

	if !data.PgpNum.IsNull() && !data.PgpNum.IsUnknown() {
		pgpNum := int(data.PgpNum.ValueInt64())
		createReq.PgpNum = &pgpNum
	}

	if !data.CrushRule.IsNull() && !data.CrushRule.IsUnknown() {
		crushRule := data.CrushRule.ValueString()
		createReq.CrushRule = &crushRule
	}

	if poolType == "erasure" {
		erasureProfile := "default"
		if !data.ErasureCodeProfile.IsNull() && !data.ErasureCodeProfile.IsUnknown() {
			erasureProfile = data.ErasureCodeProfile.ValueString()
		}
		createReq.ErasureCodeProfile = &erasureProfile
	}

	if poolType == "replicated" {
		if !data.Size.IsNull() && !data.Size.IsUnknown() {
			size := int(data.Size.ValueInt64())
			createReq.Size = &size
		}

		if !data.MinSize.IsNull() && !data.MinSize.IsUnknown() {
			minSize := int(data.MinSize.ValueInt64())
			createReq.MinSize = &minSize
		}
	}

	if !data.PgAutoscaleMode.IsNull() && !data.PgAutoscaleMode.IsUnknown() {
		pgAutoscaleMode := data.PgAutoscaleMode.ValueString()
		createReq.PgAutoscaleMode = &pgAutoscaleMode
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
		compressionMode := data.CompressionMode.ValueString()
		createReq.CompressionMode = &compressionMode
	}

	if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
		compressionAlgorithm := data.CompressionAlgorithm.ValueString()
		createReq.CompressionAlgorithm = &compressionAlgorithm
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

	if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
		var apps []string
		resp.Diagnostics.Append(data.ApplicationMetadata.ElementsAs(ctx, &apps, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.ApplicationMetadata = apps
	}

	err := r.client.CreatePool(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating pool",
			fmt.Sprintf("Unable to create pool %q: %s", data.Name.ValueString(), err),
		)
		return
	}

	pool, err := r.waitForPoolProperties(ctx, data.Name.ValueString(), &data)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pool after creation",
			fmt.Sprintf("Unable to read pool %q after creation: %s", data.Name.ValueString(), err),
		)
		return
	}

	mapPoolToModel(ctx, pool, &data, &resp.Diagnostics)

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
		if strings.Contains(err.Error(), "status 404") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading pool",
			fmt.Sprintf("Unable to read pool %q: %s", data.Name.ValueString(), err),
		)
		return
	}

	mapPoolToModel(ctx, pool, &data, &resp.Diagnostics)

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

	updateReq := CephAPIPoolUpdateRequest{}

	if !data.Name.Equal(state.Name) {
		newName := data.Name.ValueString()
		updateReq.Pool = &newName
	}

	if !data.PgNum.Equal(state.PgNum) {
		pgNum := int(data.PgNum.ValueInt64())
		if pgNum > 0 {
			updateReq.PgNum = &pgNum
		}
	}

	if !data.PgpNum.Equal(state.PgpNum) {
		pgpNum := int(data.PgpNum.ValueInt64())
		if pgpNum > 0 {
			updateReq.PgpNum = &pgpNum
		}
	}

	if !data.MinSize.Equal(state.MinSize) {
		minSize := int(data.MinSize.ValueInt64())
		updateReq.MinSize = &minSize
	}

	if !data.PgAutoscaleMode.Equal(state.PgAutoscaleMode) {
		pgAutoscaleMode := data.PgAutoscaleMode.ValueString()
		updateReq.PgAutoscaleMode = &pgAutoscaleMode
	}

	if !data.QuotaMaxObjects.Equal(state.QuotaMaxObjects) {
		quotaMaxObjects := int(data.QuotaMaxObjects.ValueInt64())
		updateReq.QuotaMaxObjects = &quotaMaxObjects
	}

	if !data.QuotaMaxBytes.Equal(state.QuotaMaxBytes) {
		quotaMaxBytes := int(data.QuotaMaxBytes.ValueInt64())
		updateReq.QuotaMaxBytes = &quotaMaxBytes
	}

	if !data.CompressionMode.Equal(state.CompressionMode) {
		if !data.CompressionMode.IsNull() && !data.CompressionMode.IsUnknown() {
			compressionMode := data.CompressionMode.ValueString()
			updateReq.CompressionMode = &compressionMode
		}
	}

	if !data.CompressionAlgorithm.Equal(state.CompressionAlgorithm) {
		if !data.CompressionAlgorithm.IsNull() && !data.CompressionAlgorithm.IsUnknown() {
			compressionAlgorithm := data.CompressionAlgorithm.ValueString()
			updateReq.CompressionAlgorithm = &compressionAlgorithm
		}
	}

	if !data.CompressionRequiredRatio.Equal(state.CompressionRequiredRatio) {
		if !data.CompressionRequiredRatio.IsNull() && !data.CompressionRequiredRatio.IsUnknown() {
			compressionRequiredRatio := data.CompressionRequiredRatio.ValueFloat64()
			updateReq.CompressionRequiredRatio = &compressionRequiredRatio
		}
	}

	if !data.CompressionMinBlobSize.Equal(state.CompressionMinBlobSize) {
		if !data.CompressionMinBlobSize.IsNull() && !data.CompressionMinBlobSize.IsUnknown() {
			compressionMinBlobSize := int(data.CompressionMinBlobSize.ValueInt64())
			updateReq.CompressionMinBlobSize = &compressionMinBlobSize
		}
	}

	if !data.CompressionMaxBlobSize.Equal(state.CompressionMaxBlobSize) {
		if !data.CompressionMaxBlobSize.IsNull() && !data.CompressionMaxBlobSize.IsUnknown() {
			compressionMaxBlobSize := int(data.CompressionMaxBlobSize.ValueInt64())
			updateReq.CompressionMaxBlobSize = &compressionMaxBlobSize
		}
	}

	if !data.ApplicationMetadata.Equal(state.ApplicationMetadata) {
		if !data.ApplicationMetadata.IsNull() && !data.ApplicationMetadata.IsUnknown() {
			var apps []string
			resp.Diagnostics.Append(data.ApplicationMetadata.ElementsAs(ctx, &apps, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			updateReq.ApplicationMetadata = apps
		}
	}

	currentName := state.Name.ValueString()

	err := r.client.UpdatePool(ctx, currentName, updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating pool",
			fmt.Sprintf("Unable to update pool %q: %s", currentName, err),
		)
		return
	}

	newName := data.Name.ValueString()
	if !data.Name.Equal(state.Name) {
		timer := time.NewTimer(poolRenameSettleTime)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
	pool, err := r.waitForPoolProperties(ctx, newName, &data)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading pool after update",
			fmt.Sprintf("Unable to read pool %q after update: %s", newName, err),
		)
		return
	}

	mapPoolToModel(ctx, pool, &data, &resp.Diagnostics)

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
			"Error deleting pool",
			fmt.Sprintf("Unable to delete pool %q: %s", data.Name.ValueString(), err),
		)
		return
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

}

func (r *PoolResource) waitForPoolProperties(ctx context.Context, poolName string, expected *PoolResourceModel) (*CephAPIPool, error) {
	tflog.Debug(ctx, "Starting to wait for pool properties", map[string]interface{}{
		"pool_name": poolName,
		"timeout":   "60s",
	})

	ctx, cancel := context.WithTimeout(ctx, poolPropertiesTimeout)
	defer cancel()

	ticker := time.NewTicker(poolPropertyCheckInterval)
	defer ticker.Stop()

	attemptCount := 0
	for {
		attemptCount++
		pool, err := r.client.GetPool(ctx, poolName)
		if err != nil {
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
				tflog.Debug(ctx, "Pool not found (404), continuing to poll", map[string]interface{}{
					"pool_name": poolName,
					"attempt":   attemptCount,
				})
				select {
				case <-ctx.Done():
					tflog.Warn(ctx, "Context timeout waiting for pool, making final attempt", map[string]interface{}{
						"pool_name": poolName,
						"attempts":  attemptCount,
					})
					return r.client.GetPool(context.Background(), poolName)
				case <-ticker.C:
					continue
				}
			}
			tflog.Error(ctx, "Error getting pool", map[string]interface{}{
				"pool_name": poolName,
				"error":     err.Error(),
				"attempt":   attemptCount,
			})
			return nil, err
		}

		propertiesMatch := true

		if !expected.Size.IsNull() && !expected.Size.IsUnknown() {
			if expected.PoolType.ValueString() == "replicated" {
				expectedSize := int(expected.Size.ValueInt64())
				if pool.Size != expectedSize {
					tflog.Debug(ctx, "Size mismatch", map[string]interface{}{
						"expected": expectedSize,
						"actual":   pool.Size,
						"attempt":  attemptCount,
					})
					propertiesMatch = false
				}
			}
		}

		if !expected.MinSize.IsNull() && !expected.MinSize.IsUnknown() {
			expectedMinSize := int(expected.MinSize.ValueInt64())
			if pool.MinSize != expectedMinSize {
				tflog.Debug(ctx, "MinSize mismatch", map[string]interface{}{
					"expected": expectedMinSize,
					"actual":   pool.MinSize,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.PgAutoscaleMode.IsNull() && !expected.PgAutoscaleMode.IsUnknown() {
			expectedMode := expected.PgAutoscaleMode.ValueString()
			if pool.PGAutoscaleMode != expectedMode {
				tflog.Debug(ctx, "PgAutoscaleMode mismatch", map[string]interface{}{
					"expected": expectedMode,
					"actual":   pool.PGAutoscaleMode,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.QuotaMaxObjects.IsNull() && !expected.QuotaMaxObjects.IsUnknown() {
			expectedQuota := int(expected.QuotaMaxObjects.ValueInt64())
			if pool.QuotaMaxObjects != expectedQuota {
				tflog.Debug(ctx, "QuotaMaxObjects mismatch", map[string]interface{}{
					"expected": expectedQuota,
					"actual":   pool.QuotaMaxObjects,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.QuotaMaxBytes.IsNull() && !expected.QuotaMaxBytes.IsUnknown() {
			expectedQuota := int(expected.QuotaMaxBytes.ValueInt64())
			if pool.QuotaMaxBytes != expectedQuota {
				tflog.Debug(ctx, "QuotaMaxBytes mismatch", map[string]interface{}{
					"expected": expectedQuota,
					"actual":   pool.QuotaMaxBytes,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.CompressionMode.IsNull() && !expected.CompressionMode.IsUnknown() {
			expectedMode := expected.CompressionMode.ValueString()
			if pool.Options.CompressionMode != expectedMode {
				tflog.Debug(ctx, "CompressionMode mismatch", map[string]interface{}{
					"expected": expectedMode,
					"actual":   pool.Options.CompressionMode,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.CompressionAlgorithm.IsNull() && !expected.CompressionAlgorithm.IsUnknown() {
			expectedAlgo := expected.CompressionAlgorithm.ValueString()
			if pool.Options.CompressionAlgorithm != expectedAlgo {
				tflog.Debug(ctx, "CompressionAlgorithm mismatch", map[string]interface{}{
					"expected": expectedAlgo,
					"actual":   pool.Options.CompressionAlgorithm,
					"attempt":  attemptCount,
				})
				propertiesMatch = false
			}
		}

		if !expected.ApplicationMetadata.IsNull() && !expected.ApplicationMetadata.IsUnknown() {
			var expectedApps []string
			diags := expected.ApplicationMetadata.ElementsAs(ctx, &expectedApps, false)
			if !diags.HasError() {
				if len(expectedApps) != len(pool.ApplicationMetadata) {
					tflog.Debug(ctx, "ApplicationMetadata length mismatch", map[string]interface{}{
						"expected_count": len(expectedApps),
						"actual_count":   len(pool.ApplicationMetadata),
						"expected_apps":  expectedApps,
						"actual_apps":    pool.ApplicationMetadata,
						"attempt":        attemptCount,
					})
					propertiesMatch = false
				} else {
					for _, app := range expectedApps {
						found := false
						for _, poolApp := range pool.ApplicationMetadata {
							if app == poolApp {
								found = true
								break
							}
						}
						if !found {
							tflog.Debug(ctx, "ApplicationMetadata missing expected app", map[string]interface{}{
								"missing_app":   app,
								"expected_apps": expectedApps,
								"actual_apps":   pool.ApplicationMetadata,
								"attempt":       attemptCount,
							})
							propertiesMatch = false
							break
						}
					}
				}
			}
		}

		if propertiesMatch {
			tflog.Debug(ctx, "All properties matched, returning pool", map[string]interface{}{
				"pool_name": poolName,
				"attempt":   attemptCount,
			})
			return pool, nil
		}

		select {
		case <-ctx.Done():
			tflog.Warn(ctx, "Polling loop ending due to context timeout", map[string]interface{}{
				"pool_name": poolName,
				"attempts":  attemptCount,
			})
			return r.client.GetPool(context.Background(), poolName)
		case <-ticker.C:
		}
	}
}

func mapPoolToModel(ctx context.Context, pool *CephAPIPool, data *PoolResourceModel, diagnostics *diag.Diagnostics) {
	data.Name = types.StringValue(pool.PoolName)
	data.PoolType = types.StringValue(pool.Type)
	data.PoolID = types.Int64Value(int64(pool.PoolID))
	data.Size = types.Int64Value(int64(pool.Size))
	data.MinSize = types.Int64Value(int64(pool.MinSize))
	data.PgNum = types.Int64Value(int64(pool.PGNum))
	data.PgpNum = types.Int64Value(int64(pool.PGPlacementNum))
	data.CrushRule = types.StringValue(pool.CrushRule)
	data.PgAutoscaleMode = types.StringValue(pool.PGAutoscaleMode)
	data.PrimaryAffinity = types.Float64Value(pool.PrimaryAffinity)
	data.QuotaMaxObjects = types.Int64Value(int64(pool.QuotaMaxObjects))
	data.QuotaMaxBytes = types.Int64Value(int64(pool.QuotaMaxBytes))

	if pool.ErasureCodeProfile != "" {
		data.ErasureCodeProfile = types.StringValue(pool.ErasureCodeProfile)
	} else {
		data.ErasureCodeProfile = types.StringNull()
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
		apps, diags := types.ListValueFrom(ctx, types.StringType, pool.ApplicationMetadata)
		diagnostics.Append(diags...)
		data.ApplicationMetadata = apps
	} else {
		data.ApplicationMetadata = types.ListNull(types.StringType)
	}
}
