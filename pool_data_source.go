package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &PoolDataSource{}

func newPoolDataSource() datasource.DataSource {
	return &PoolDataSource{}
}

type PoolDataSource struct {
	client *restapi.Client
}

type PoolDataSourceModel struct {
	Name                     types.String  `tfsdk:"name"`
	PoolID                   types.Int64   `tfsdk:"pool_id"`
	Size                     types.Int64   `tfsdk:"size"`
	MinSize                  types.Int64   `tfsdk:"min_size"`
	PGNum                    types.Int64   `tfsdk:"pg_num"`
	CrushRule                types.String  `tfsdk:"crush_rule"`
	ApplicationMetadata      types.Set     `tfsdk:"application_metadata"`
	Flags                    types.Int64   `tfsdk:"flags"`
	FlagsNames               types.String  `tfsdk:"flags_names"`
	ErasureCodeProfile       types.String  `tfsdk:"erasure_code_profile"`
	AutoscaleMode            types.String  `tfsdk:"autoscale_mode"`
	QuotaMaxObjects          types.Int64   `tfsdk:"quota_max_objects"`
	QuotaMaxBytes            types.Int64   `tfsdk:"quota_max_bytes"`
	CompressionMode          types.String  `tfsdk:"compression_mode"`
	CompressionAlgorithm     types.String  `tfsdk:"compression_algorithm"`
	CompressionRequiredRatio types.Float64 `tfsdk:"compression_required_ratio"`
	CompressionMinBlobSize   types.Int64   `tfsdk:"compression_min_blob_size"`
	CompressionMaxBlobSize   types.Int64   `tfsdk:"compression_max_blob_size"`
	PgNumMin                 types.Int64   `tfsdk:"pg_num_min"`
	PgNumMax                 types.Int64   `tfsdk:"pg_num_max"`
	TargetSizeRatio          types.Float64 `tfsdk:"target_size_ratio"`
	TargetSizeBytes          types.Int64   `tfsdk:"target_size_bytes"`
	FastRead                 types.Bool    `tfsdk:"fast_read"`
	Configuration            types.Map     `tfsdk:"configuration"`
}

func (d *PoolDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *PoolDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph pool.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the pool.",
				Required:            true,
			},
			"pool_id": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The ID of the pool.",
				Computed:            true,
			},
			"size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The number of replicas for the pool.",
				Computed:            true,
			},
			"min_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The minimum number of replicas for the pool.",
				Computed:            true,
			},
			"pg_num": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The number of placement groups for the pool.",
				Computed:            true,
			},
			"crush_rule": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH rule for the pool.",
				Computed:            true,
			},
			"application_metadata": dataSourceSchema.SetAttribute{
				MarkdownDescription: "The list of applications enabled on the pool (e.g. 'rbd', 'rgw', 'cephfs').",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"flags_names": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The names of the flags set on the pool, comma separated.",
				Computed:            true,
			},
			"flags": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The flags of the pool.",
				Computed:            true,
			},
			"erasure_code_profile": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The erasure code profile of the pool.",
				Computed:            true,
			},
			"autoscale_mode": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The autoscale mode of the pool.",
				Computed:            true,
			},
			"quota_max_objects": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of objects allowed in the pool (hard limit).",
				Computed:            true,
			},
			"quota_max_bytes": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum bytes allowed in the pool (hard limit).",
				Computed:            true,
			},
			"compression_mode": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The compression mode of the pool.",
				Computed:            true,
			},
			"compression_algorithm": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The compression algorithm of the pool.",
				Computed:            true,
			},
			"compression_required_ratio": dataSourceSchema.Float64Attribute{
				MarkdownDescription: "The compression required ratio of the pool.",
				Computed:            true,
			},
			"compression_min_blob_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The compression minimum blob size of the pool.",
				Computed:            true,
			},
			"compression_max_blob_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The compression maximum blob size of the pool.",
				Computed:            true,
			},
			"pg_num_min": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The minimum number of placement groups the autoscaler may shrink the pool to.",
				Computed:            true,
			},
			"pg_num_max": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum number of placement groups the autoscaler may grow the pool to.",
				Computed:            true,
			},
			"target_size_ratio": dataSourceSchema.Float64Attribute{
				MarkdownDescription: "The expected share of total cluster capacity this pool will consume.",
				Computed:            true,
			},
			"target_size_bytes": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The expected size of this pool in bytes.",
				Computed:            true,
			},
			"fast_read": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether reads issue parallel requests to all shards and reconstruct from the first arrivals. Always false for replicated pools.",
				Computed:            true,
			},
			"configuration": dataSourceSchema.MapAttribute{
				MarkdownDescription: "Pool-level RBD configuration overrides.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *PoolDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *PoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PoolDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	pool, err := d.client.GetPool(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get pool '%s' from Ceph API: %s", data.Name.ValueString(), err),
		)
		return
	}

	data.PoolID = types.Int64Value(int64(pool.PoolID))
	data.Size = types.Int64Value(int64(pool.Size))
	data.MinSize = types.Int64Value(int64(pool.MinSize))
	data.PGNum = types.Int64Value(int64(pool.PGNum))
	data.CrushRule = types.StringValue(pool.CrushRule)
	data.ErasureCodeProfile = types.StringValue(pool.ErasureCodeProfile)
	data.AutoscaleMode = types.StringValue(pool.PGAutoscaleMode)
	data.QuotaMaxObjects = types.Int64Value(int64(pool.QuotaMaxObjects))
	data.QuotaMaxBytes = types.Int64Value(int64(pool.QuotaMaxBytes))
	data.CompressionMode = types.StringValue(pool.Options.CompressionMode)
	data.CompressionAlgorithm = types.StringValue(pool.Options.CompressionAlgorithm)
	data.CompressionRequiredRatio = types.Float64Value(pool.Options.CompressionRequiredRatio)
	data.CompressionMinBlobSize = types.Int64Value(int64(pool.Options.CompressionMinBlobSize))
	data.CompressionMaxBlobSize = types.Int64Value(int64(pool.Options.CompressionMaxBlobSize))
	if pool.Options.PgNumMin > 0 {
		data.PgNumMin = types.Int64Value(int64(pool.Options.PgNumMin))
	} else {
		data.PgNumMin = types.Int64Null()
	}
	if pool.Options.PgNumMax > 0 {
		data.PgNumMax = types.Int64Value(int64(pool.Options.PgNumMax))
	} else {
		data.PgNumMax = types.Int64Null()
	}
	if pool.Options.TargetSizeRatio > 0 {
		data.TargetSizeRatio = types.Float64Value(pool.Options.TargetSizeRatio)
	} else {
		data.TargetSizeRatio = types.Float64Null()
	}
	if pool.Options.TargetSizeBytes > 0 {
		data.TargetSizeBytes = types.Int64Value(pool.Options.TargetSizeBytes)
	} else {
		data.TargetSizeBytes = types.Int64Null()
	}
	data.FastRead = types.BoolValue(pool.FastRead)

	data.Flags = types.Int64Value(int64(pool.Flags))
	data.FlagsNames = types.StringValue(pool.FlagsNames)

	poolConfig := map[string]string{}
	for _, option := range pool.Configuration {
		// Source 1 marks pool-level overrides.
		if option.Source == 1 {
			poolConfig[option.Name] = option.Value
		}
	}
	configuration, diags := types.MapValueFrom(ctx, types.StringType, poolConfig)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Configuration = configuration

	appMetaStrings := pool.ApplicationMetadata
	appMeta, diags := types.SetValueFrom(ctx, types.StringType, appMetaStrings)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ApplicationMetadata = appMeta

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
