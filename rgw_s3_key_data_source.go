package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RGWS3KeyDataSource{}

func newRGWS3KeyDataSource() datasource.DataSource {
	return &RGWS3KeyDataSource{}
}

type RGWS3KeyDataSource struct {
	client *CephAPIClient
}

type RGWS3KeyDataSourceModel struct {
	User       types.String `tfsdk:"user"`
	AccessKey  types.String `tfsdk:"access_key"`
	SecretKey  types.String `tfsdk:"secret_key"`
	Active     types.Bool   `tfsdk:"active"`
	CreateDate types.String `tfsdk:"create_date"`
}

func (d *RGWS3KeyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_s3_key"
}

func (d *RGWS3KeyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW S3 access key.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"user": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The user or subuser ID that owns the S3 key (format: 'user' or 'user:subuser')",
				Required:            true,
			},
			"access_key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The S3 access key ID to look up (required if user has multiple S3 keys)",
				Optional:            true,
			},
			"secret_key": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The S3 secret key",
				Computed:            true,
				Sensitive:           true,
			},
			"active": dataSourceSchema.BoolAttribute{
				MarkdownDescription: "Whether the key is active",
				Computed:            true,
			},
			"create_date": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The creation date of the key",
				Computed:            true,
			},
		},
	}
}

func (d *RGWS3KeyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *RGWS3KeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWS3KeyDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	userID := data.User.ValueString()
	accessKey := data.AccessKey.ValueString()

	parts := strings.SplitN(userID, ":", 2)
	parentUID := parts[0]

	user, err := d.client.RGWGetUser(ctx, parentUID)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW user from Ceph API: %s", err),
		)
		return
	}

	var matchingKeys []CephAPIRGWS3Key
	for i := range user.Keys {
		if user.Keys[i].User == userID {
			matchingKeys = append(matchingKeys, user.Keys[i])
		}
	}

	if accessKey != "" {
		var filteredKeys []CephAPIRGWS3Key
		for i := range matchingKeys {
			if matchingKeys[i].AccessKey == accessKey {
				filteredKeys = append(filteredKeys, matchingKeys[i])
			}
		}
		matchingKeys = filteredKeys
	}

	if len(matchingKeys) == 0 {
		if accessKey != "" {
			resp.Diagnostics.AddError(
				"Key Not Found",
				fmt.Sprintf("S3 access key %s not found for user %s", accessKey, userID),
			)
		} else {
			resp.Diagnostics.AddError(
				"Key Not Found",
				fmt.Sprintf("No S3 keys found for user %s", userID),
			)
		}
		return
	}

	if len(matchingKeys) > 1 {
		resp.Diagnostics.AddError(
			"Multiple Keys Found",
			fmt.Sprintf("User %s has %d S3 keys. Please specify the access_key parameter to disambiguate.", userID, len(matchingKeys)),
		)
		return
	}

	foundKey := matchingKeys[0]
	data.AccessKey = types.StringValue(foundKey.AccessKey)
	data.SecretKey = types.StringValue(foundKey.SecretKey)
	data.Active = types.BoolValue(foundKey.Active)
	if foundKey.CreateDate != "" {
		data.CreateDate = types.StringValue(foundKey.CreateDate)
	} else {
		data.CreateDate = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
