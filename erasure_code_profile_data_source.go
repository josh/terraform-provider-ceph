package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ErasureCodeProfileDataSource{}

func newErasureCodeProfileDataSource() datasource.DataSource {
	return &ErasureCodeProfileDataSource{}
}

type ErasureCodeProfileDataSource struct {
	client *CephAPIClient
}

type ErasureCodeProfileDataSourceModel struct {
	Name                      types.String `tfsdk:"name"`
	K                         types.Int64  `tfsdk:"k"`
	M                         types.Int64  `tfsdk:"m"`
	Plugin                    types.String `tfsdk:"plugin"`
	CrushFailureDomain        types.String `tfsdk:"crush_failure_domain"`
	CrushMinFailureDomain     types.Int64  `tfsdk:"crush_min_failure_domain"`
	CrushOsdsPerFailureDomain types.Int64  `tfsdk:"crush_osds_per_failure_domain"`
	PacketSize                types.Int64  `tfsdk:"packet_size"`
	Technique                 types.String `tfsdk:"technique"`
	CrushRoot                 types.String `tfsdk:"crush_root"`
	CrushDeviceClass          types.String `tfsdk:"crush_device_class"`
	Directory                 types.String `tfsdk:"directory"`
}

func (d *ErasureCodeProfileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_erasure_code_profile"
}

func (d *ErasureCodeProfileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about an erasure code profile.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the erasure code profile",
				Required:            true,
			},
			"k": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Number of data chunks",
				Computed:            true,
			},
			"m": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Number of coding chunks",
				Computed:            true,
			},
			"plugin": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The erasure code plugin (e.g., jerasure, shec, clay)",
				Computed:            true,
			},
			"crush_failure_domain": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH failure domain for placement",
				Computed:            true,
			},
			"crush_min_failure_domain": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Minimum number of CRUSH failure domains required (Ceph 'crush-min-size').",
				Computed:            true,
			},
			"crush_osds_per_failure_domain": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Number of OSDs placed per CRUSH failure domain (Ceph 'crush-osds-per-failure-domain').",
				Computed:            true,
			},
			"packet_size": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "Packet size used by the erasure coding plugin.",
				Computed:            true,
			},
			"technique": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The encoding technique used",
				Computed:            true,
			},
			"crush_root": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The CRUSH root for placement",
				Computed:            true,
			},
			"crush_device_class": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The device class for placement",
				Computed:            true,
			},
			"directory": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The directory where the plugin is loaded from",
				Computed:            true,
			},
		},
	}
}

func (d *ErasureCodeProfileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *ErasureCodeProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ErasureCodeProfileDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()

	profile, err := d.client.GetErasureCodeProfile(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get erasure code profile '%s' from Ceph API: %s", name, err),
		)
		return
	}

	data.K = types.Int64Value(int64(profile.K))
	data.M = types.Int64Value(int64(profile.M))
	data.Plugin = types.StringValue(profile.Plugin)
	data.CrushFailureDomain = types.StringValue(profile.CrushFailureDomain)
	if profile.CrushMinFailureDomain != nil {
		data.CrushMinFailureDomain = types.Int64Value(int64(*profile.CrushMinFailureDomain))
	} else {
		data.CrushMinFailureDomain = types.Int64Null()
	}
	if profile.CrushOsdsPerFailureDomain != nil {
		data.CrushOsdsPerFailureDomain = types.Int64Value(int64(*profile.CrushOsdsPerFailureDomain))
	} else {
		data.CrushOsdsPerFailureDomain = types.Int64Null()
	}
	if profile.PacketSize != nil {
		data.PacketSize = types.Int64Value(int64(*profile.PacketSize))
	} else {
		data.PacketSize = types.Int64Null()
	}
	if profile.Technique != "" {
		data.Technique = types.StringValue(profile.Technique)
	} else {
		data.Technique = types.StringNull()
	}
	if profile.CrushRoot != "" {
		data.CrushRoot = types.StringValue(profile.CrushRoot)
	} else {
		data.CrushRoot = types.StringNull()
	}
	if profile.CrushDeviceClass != "" {
		data.CrushDeviceClass = types.StringValue(profile.CrushDeviceClass)
	} else {
		data.CrushDeviceClass = types.StringNull()
	}
	data.Directory = types.StringValue(profile.Directory)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
